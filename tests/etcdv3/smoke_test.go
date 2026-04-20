// Package etcdv3_test will run integration tests for using etcd as backend with v3 api
package etcdv3_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/docker/go-connections/nat"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/network"
)

var _ = Describe("Smoke", Ordered, func() {
	const (
		numEtcd       = 1
		autoRemove    = false
		initialConfig = `{"stolon_custom_config":{"defaultSUReplAccessMode":"strict","pgParameters":{},"pgHBA":[]}}`

		numKeepers = 3

		pgPassword = "test123"
	)
	var (
		ctx              context.Context
		nw               *testcontainers.DockerNetwork
		etcdContainer    *etcd.EtcdContainer
		etcdEndpoints    string
		sentinelCnt      testcontainers.Container
		proxyCnt         testcontainers.Container
		keeperContainers []testcontainers.Container
		allContainers    []testcontainers.Container
		keeperSettings   = map[string]string{
			"pg-repl-password": pgPassword,
			"pg-su-password":   pgPassword,
		}
		pgConn = pgConnParams{
			"host":     "localhost",
			"user":     pgUser,
			"password": pgPassword,
			"dbname":   pgDatabase,
		}
	)

	BeforeAll(func() {
		// RYUK requires permissions we don't need and don't want to implement
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

		ctx = context.Background()

		var nwErr error
		nw, nwErr = network.New(ctx)
		Ω(nwErr).NotTo(HaveOccurred())

		// setup etcd
		var etcdErr error
		etcdContainer, etcdEndpoints, etcdErr = runEtcd(ctx, etcdImage, nw)
		Ω(etcdErr).NotTo(HaveOccurred())
		allContainers = []testcontainers.Container{etcdContainer}

		// run stolonctl to define a new cluster in etcd
		stolonCtlInitCnt, initErr := runStolonCtl(
			ctx,
			etcdEndpoints,
			nw,
			"init", "--yes")
		Ω(initErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, stolonCtlInitCnt)

		// Run stolonctl patch to set initial config
		stolonCtlPatchCnt, patchErr := runStolonCtl(
			ctx,
			etcdEndpoints,
			nw,
			"update",
			"--patch",
			initialConfig,
		)
		Ω(patchErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, stolonCtlPatchCnt)
		// - cat myspec.json | stolonctl update --patch --file -
		//   or
		// - stolonctl update --patch "${MYSPEC}"
		//   or
		// - stolonctl update --patch --file "${STOLONCTL_FILE}"'

		// Start sentinel
		var sentinelErr error
		sentinelCnt, sentinelErr = runSentinel(ctx, etcdEndpoints, nw)
		Ω(sentinelErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, sentinelCnt)

		// start keeper(s)
		for i := 0; i < numKeepers; i++ {
			alias := fmt.Sprintf("keeper_%d", i)
			settings := maps.Clone(keeperSettings)
			settings["pg-listen-address"] = alias
			aliases := map[string][]string{}
			aliases[nw.Name] = []string{alias}
			cnt, keeperErr := runKeeper(ctx, etcdEndpoints, nw, aliases, settings)
			Ω(keeperErr).NotTo(HaveOccurred())
			keeperContainers = append(keeperContainers, cnt)
			allContainers = append(allContainers, cnt)
		}

		// Start proxy
		var proxyErr error
		aliases := map[string][]string{}
		aliases[nw.Name] = []string{"proxy"}
		proxyCnt, proxyErr = runProxy(ctx, etcdEndpoints, nw, aliases)
		Ω(proxyErr).NotTo(HaveOccurred())
		allContainers = append(allContainers, proxyCnt)

		/*
			logs, logErr := cnt.Logs(ctx)
			Ω(logErr).NotTo(HaveOccurred())
			data, readErr := io.ReadAll(logs)
			Ω(readErr).NotTo(HaveOccurred())
			fmt.Fprintf(GinkgoWriter, "DEBUG - Logs: %s", string(data))
		*/
		// wait for postgres to be available
	})
	AfterAll(func() {
		if !autoRemove {
			return
		}
		if CurrentSpecReport().Failed() {
			GinkgoWriter.Printf("Test failed! not cleaning containers")
			return
		}
		for _, cnt := range allContainers {
			Ω(cnt.Terminate(ctx)).NotTo(HaveOccurred())
		}
		Ω(nw.Remove(ctx)).NotTo(HaveOccurred())
	})
	Context("when connecting to the keepers", func() {
		It("should work properly", func() {
			for _, cnt := range keeperContainers {
				natPort, err := cnt.MappedPort(ctx,
					nat.Port(fmt.Sprintf("%d/tcp", keeperPort)))
				Ω(err).NotTo(HaveOccurred())
				Ω(pgPing(
					ctx,
					pgConn.setParam("port", natPort.Port())),
				).NotTo(HaveOccurred())
			}
		})
	})
	Context("when connecting through proxy", func() {
		It("should work properly", func() {
			natPort, err := proxyCnt.MappedPort(ctx,
				nat.Port(fmt.Sprintf("%d/tcp", proxyPort)))
			Ω(err).NotTo(HaveOccurred())
			proxyConnSettings := pgConn.setParam("port", natPort.Port())
			// This does not work directly after starting the container but does after 5s.
			// So, we will try this for 10 seconds
			isReadyCtx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(time.Second*10))
			defer cancelFunc()
			// every 100 miliseconds
			isReadyErr := isReady(isReadyCtx, proxyConnSettings, time.Millisecond*100)
			Ω(isReadyErr).NotTo(HaveOccurred())
		})
	})
})
