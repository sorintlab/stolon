package cluster

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/sorintlab/stolon/internal/util"
)

const (
	maxPgVersion = 20
)

var (
	testInitMode         = InitMode("test")
	testRole             = Role("test")
	testSUReplAccessMode = SUReplAccessMode("test")
	testHBA              = []string{
		"local all all deny",
		"host all all 0.0.0.0/0 deny",
	}
	testSettings = PGParameters{"min_wal_keep": "1G"}
)

func randomInt() int        { return rand.Int() }
func randomInt64() int64    { return rand.Int63() }
func randomUInt16() *uint16 { var i = uint16(rand.Uint32() % 1048576); return &i }
func randomUInt32() *uint32 { var i = rand.Uint32(); return &i }
func randomUInt64() uint64  { return rand.Uint64() }
func randomString() string  { return fmt.Sprintf("%x", rand.Int63()) }
func randomBool() bool      { return (rand.Int()%2 == 1) }
func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Uint32()%256, rand.Uint32()%256,
		rand.Uint32()%256, rand.Uint32()%256)
}
func randomPort() string { return fmt.Sprintf("%d", rand.Uint32()) }

const maxNano uint64 = 1000 * 1000 * 1000 * 3600

func randomDuration() *Duration         { return &Duration{Duration: randomTimeDuration()} }
func randomTimeDuration() time.Duration { return time.Duration(rand.Uint64() % maxNano) }

func randomTLH() *PostgresTimelineHistory {
	return &PostgresTimelineHistory{
		TimelineID:  rand.Uint64(),
		SwitchPoint: rand.Uint64(),
		Reason:      randomString(),
	}
}
func randomTLSH(num uint) PostgresTimelinesHistory {
	var pths PostgresTimelinesHistory
	for i := uint(0); i < num; i++ {
		pths = append(pths, randomTLH())
	}
	return pths
}

func randomCluster() *Cluster {
	return &Cluster{
		UID:        randomString(),
		Generation: randomInt64(),
		Spec:       randomSpec(),
	}
}

func randomSpec() *Spec {
	return &Spec{
		SleepInterval:             randomDuration(),
		RequestTimeout:            randomDuration(),
		ConvergenceTimeout:        randomDuration(),
		InitTimeout:               randomDuration(),
		SyncTimeout:               randomDuration(),
		DBWaitReadyTimeout:        randomDuration(),
		FailInterval:              randomDuration(),
		DeadKeeperRemovalInterval: randomDuration(),
		ProxyCheckInterval:        randomDuration(),
		ProxyTimeout:              randomDuration(),
		MaxStandbys:               randomUInt16(),
		MaxStandbysPerSender:      randomUInt16(),
		MaxStandbyLag:             randomUInt32(),
		SynchronousReplication:    util.ToPtr(randomBool()),
		MinSynchronousStandbys:    randomUInt16(),
		MaxSynchronousStandbys:    randomUInt16(),
		AdditionalWalSenders:      randomUInt16(),
		AdditionalMasterReplicationSlots: []string{
			randomString(),
		},
		UsePgrewind:       util.ToPtr(randomBool()),
		InitMode:          &testInitMode,
		MergePgParameters: util.ToPtr(randomBool()),
		Role:              &testRole,
		NewConfig: &NewConfig{
			Locale:        "en/us",
			Encoding:      "utf8",
			DataChecksums: true,
		},
		DefaultSUReplAccessMode: &testSUReplAccessMode,
		PGParameters:            testSettings,
		PGHBA:                   testHBA,
		AutomaticPgRestart:      util.ToPtr(randomBool()),
	}
}

func randomData() *Data {
	return &Data{
		FormatVersion: randomUInt64(),
		ChangeTime:    time.Now(),
		Cluster:       randomCluster(),
		Keepers:       randomKeepers(3),
		DBs:           nil,
		Proxy:         nil,
	}
}

func randomKeepers(num uint) Keepers {
	ks := Keepers{}
	for i := 0; i < int(num); i++ {
		k := randomKeeper()
		ks[k.UID] = &k
	}
	return ks
}

func randomKeeper() Keeper {
	return Keeper{
		UID:        randomString(),
		Generation: randomInt64(),
		ChangeTime: time.Now(),
		Spec:       randomKeeperSpec(),
		Status:     KeeperStatus{},
	}
}

func randomKeeperSpec() *KeeperSpec {
	return &KeeperSpec{}
}

func randomKeeperInfo() *KeeperInfo {
	return &KeeperInfo{
		InfoUID: randomString(),

		UID:        randomString(),
		ClusterUID: randomString(),
		BootUUID:   randomString(),

		PostgresBinaryVersion: randomPostgresBinaryVersion(),

		PostgresState: nil,

		CanBeMaster:             util.ToPtr(randomBool()),
		CanBeSynchronousReplica: util.ToPtr(randomBool()),
	}
}

func randomKeepersInfo(num uint) KeepersInfo {
	kis := KeepersInfo{}
	for i := 0; i < int(num); i++ {
		ki := randomKeeperInfo()
		kis[ki.UID] = ki
	}

	return kis
}
func randomPostgresBinaryVersion() PostgresBinaryVersion {
	return PostgresBinaryVersion{
		Maj: rand.Int() % maxPgVersion,
		Min: rand.Int() % maxPgVersion,
	}
}

func randomPostgresState() *PostgresState {
	return &PostgresState{
		UID:        randomString(),
		Generation: randomInt64(),

		ListenAddress: randomIP(),
		Port:          randomPort(),

		Healthy: randomBool(),

		SystemID:         randomString(),
		TimelineID:       randomUInt64(),
		XLogPos:          randomUInt64(),
		TimelinesHistory: randomTLSH(3),

		PGParameters: nil,
		SynchronousStandbys: []string{
			randomString(),
			randomString(),
		},
		OlderWalFile: randomString(),
	}
}

func randomProxyInfo() *ProxyInfo {
	return &ProxyInfo{
		InfoUID: randomString(),

		UID:          randomString(),
		Generation:   randomInt64(),
		ProxyTimeout: randomTimeDuration(),
	}
}

func randomProxiesInfo(num uint) ProxiesInfo {
	pis := ProxiesInfo{}
	for i := 0; i < int(num); i++ {
		pi := randomProxyInfo()
		pis[pi.UID] = pi
	}

	return pis
}

func randomSentinelInfo() *SentinelInfo {
	return &SentinelInfo{
		UID: randomString(),
	}
}
func randomSentinelsInfo(num uint) SentinelsInfo {
	ssi := SentinelsInfo{}
	for i := 0; i < int(num); i++ {
		ssi = append(ssi, randomSentinelInfo())
	}
	return ssi
}
