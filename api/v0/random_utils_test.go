package v0

import (
	"fmt"
	"math/rand"
)

func randomInt() int       { return rand.Int() }
func randomUInt64() uint64 { return rand.Uint64() }
func randomString() string { return fmt.Sprintf("%x", rand.Int63()) }
func randomBool() bool     { return (rand.Int()%2 == 1) }
func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Uint32()%256, rand.Uint32()%256,
		rand.Uint32()%256, rand.Uint32()%256)
}
func randomPort() string { return fmt.Sprintf("%d", rand.Uint32()) }

func randomKeeperInfo() *KeeperInfo {
	return &KeeperInfo{
		ID:                 randomString(),
		ClusterViewVersion: rand.Int(),
		ListenAddress:      randomString(),
		Port:               randomPort(),
		PGListenAddress:    randomIP(),
		PGPort:             randomPort(),
	}
}

func randomKeeperState() *KeeperState {
	return &KeeperState{
		ID:                 randomString(),
		Healthy:            randomBool(),
		ClusterViewVersion: rand.Int(),
		ListenAddress:      randomIP(),
		Port:               randomPort(),
		PGListenAddress:    randomIP(),
		PGPort:             randomPort(),
		PGState:            &PostgresState{},
	}
}
func randomKeepersState(num uint) KeepersState {
	kss := KeepersState{}
	for i := uint(0); i < num; i++ {
		ks := randomKeeperState()
		kss[ks.ID] = ks
	}
	return kss
}

func randomTLH() *PostgresTimelineHistory {
	return &PostgresTimelineHistory{
		TimelineID:  rand.Uint64(),
		SwitchPoint: rand.Uint64(),
		Reason:      randomString(),
	}
}
func randomTLSH() PostgresTimelinesHistory {
	return PostgresTimelinesHistory{
		randomTLH(),
	}
}

func randomKeeperRole() *KeeperRole {
	return &KeeperRole{
		ID:     randomString(),
		Follow: randomString(),
	}
}

func randomKeepersRole(num uint) KeepersRole {
	ksr := KeepersRole{}
	for i := uint(0); i < num; i++ {
		kr := randomKeeperRole()
		ksr[kr.ID] = kr
	}
	return ksr
}

func randomProxyConf() *ProxyConf {
	return &ProxyConf{
		Host: randomIP(),
		Port: randomPort(),
	}
}

func randomClusterView() *ClusterView {
	var c NilConfig
	c.MergeDefaults()
	return &ClusterView{
		Version:     randomInt(),
		Master:      randomString(),
		KeepersRole: randomKeepersRole(3),
		ProxyConf:   randomProxyConf(),
		Config:      &c,
	}
}
