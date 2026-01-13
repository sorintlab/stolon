package cluster

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sorintlab/stolon/internal/util"
)

var _ = Describe("Cluster", func() {
	When("Copying a Cluster", func() {
		var (
			orgCluster, newCluster *Cluster
		)
		BeforeEach(func() {
			orgCluster = randomCluster()
			newCluster = orgCluster.DeepCopy()
		})
		It("should be a copy", func() {
			newCluster.Generation = randomInt64()
			Ω(newCluster).NotTo(Equal(orgCluster))
		})
		It("should copy all fields", func() {
			Ω(newCluster).To(Equal(orgCluster))
		})
		It("should return nil when Cluster is nil", func() {
			var c *Cluster
			Ω(c.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Cluster is empty struct", func() {
			var c Cluster
			Ω(c.DeepCopy()).To(Equal(&Cluster{}))
		})
	})
	When("Requesting a cluster with a Default Spec", func() {
		It("should be a copy", func() {
			c := randomCluster()
			spec := c.DefSpec()
			spec.MaxStandbyLag = randomUInt32()
			Ω(c.Spec.MaxStandbyLag).NotTo(Equal(spec.MaxStandbyLag))
		})
		It("should set defaults for unspecified values", func() {
			c := Cluster{}
			spec := c.DefSpec()
			Ω(spec.SleepInterval).To(Equal(&Duration{Duration: DefaultSleepInterval}))
			Ω(spec.RequestTimeout).To(Equal(&Duration{Duration: DefaultRequestTimeout}))
			Ω(spec.ConvergenceTimeout).To(Equal(&Duration{Duration: DefaultConvergenceTimeout}))
			Ω(spec.InitTimeout).To(Equal(&Duration{Duration: DefaultInitTimeout}))
			Ω(spec.SyncTimeout).To(Equal(&Duration{Duration: DefaultSyncTimeout}))
			Ω(spec.DBWaitReadyTimeout).To(Equal(&Duration{Duration: DefaultDBWaitReadyTimeout}))
			Ω(spec.FailInterval).To(Equal(&Duration{Duration: DefaultFailInterval}))
			Ω(spec.DeadKeeperRemovalInterval).To(Equal(&Duration{Duration: DefaultDeadKeeperRemovalInterval}))
			Ω(spec.ProxyCheckInterval).To(Equal(&Duration{Duration: DefaultProxyCheckInterval}))
			Ω(spec.ProxyTimeout).To(Equal(&Duration{Duration: DefaultProxyTimeout}))
			Ω(spec.MaxStandbys).To(Equal(util.ToPtr(DefaultMaxStandbys)))
			Ω(spec.MaxStandbysPerSender).To(Equal(util.ToPtr(DefaultMaxStandbysPerSender)))
			Ω(spec.MaxStandbyLag).To(Equal(util.ToPtr(uint32(DefaultMaxStandbyLag))))
			Ω(spec.SynchronousReplication).To(Equal(util.ToPtr(DefaultSynchronousReplication)))
			Ω(spec.UsePgrewind).To(Equal(util.ToPtr(DefaultUsePgrewind)))
			Ω(spec.MinSynchronousStandbys).To(Equal(util.ToPtr(DefaultMinSynchronousStandbys)))
			Ω(spec.MaxSynchronousStandbys).To(Equal(util.ToPtr(DefaultMaxSynchronousStandbys)))
			Ω(spec.AdditionalWalSenders).To(Equal(util.ToPtr(uint16(DefaultAdditionalWalSenders))))
			Ω(spec.MergePgParameters).To(Equal(util.ToPtr(DefaultMergePGParameter)))
			Ω(spec.DefaultSUReplAccessMode).To(Equal(util.ToPtr(DefaultSUReplAccess)))
			Ω(spec.Role).To(Equal(util.ToPtr(DefaultRole)))
			Ω(spec.AutomaticPgRestart).To(Equal(util.ToPtr(DefaultAutomaticPgRestart)))
		})
		It("should not set defaults for specified values", func() {
			c := randomCluster()
			spec := c.DefSpec()
			Ω(spec.SleepInterval).To(Equal(c.Spec.SleepInterval))
			Ω(spec.RequestTimeout).To(Equal(c.Spec.RequestTimeout))
			Ω(spec.ConvergenceTimeout).To(Equal(c.Spec.ConvergenceTimeout))
			Ω(spec.InitTimeout).To(Equal(c.Spec.InitTimeout))
			Ω(spec.SyncTimeout).To(Equal(c.Spec.SyncTimeout))
			Ω(spec.DBWaitReadyTimeout).To(Equal(c.Spec.DBWaitReadyTimeout))
			Ω(spec.FailInterval).To(Equal(c.Spec.FailInterval))
			Ω(spec.DeadKeeperRemovalInterval).To(Equal(c.Spec.DeadKeeperRemovalInterval))
			Ω(spec.ProxyCheckInterval).To(Equal(c.Spec.ProxyCheckInterval))
			Ω(spec.ProxyTimeout).To(Equal(c.Spec.ProxyTimeout))
			Ω(spec.MaxStandbys).To(Equal(c.Spec.MaxStandbys))
			Ω(spec.MaxStandbysPerSender).To(Equal(c.Spec.MaxStandbysPerSender))
			Ω(spec.MaxStandbyLag).To(Equal(c.Spec.MaxStandbyLag))
			Ω(spec.SynchronousReplication).To(Equal(c.Spec.SynchronousReplication))
			Ω(spec.UsePgrewind).To(Equal(c.Spec.UsePgrewind))
			Ω(spec.MinSynchronousStandbys).To(Equal(c.Spec.MinSynchronousStandbys))
			Ω(spec.MaxSynchronousStandbys).To(Equal(c.Spec.MaxSynchronousStandbys))
			Ω(spec.AdditionalWalSenders).To(Equal(c.Spec.AdditionalWalSenders))
			Ω(spec.MergePgParameters).To(Equal(c.Spec.MergePgParameters))
			Ω(spec.DefaultSUReplAccessMode).To(Equal(c.Spec.DefaultSUReplAccessMode))
			Ω(spec.Role).To(Equal(c.Spec.Role))
			Ω(spec.AutomaticPgRestart).To(Equal(c.Spec.AutomaticPgRestart))
		})
	})
	When("Copying a Spec", func() {
		var (
			orgSpec, newSpec *Spec
		)
		BeforeEach(func() {
			orgSpec = randomSpec()
			newSpec = orgSpec.DeepCopy()
		})
		It("should be a copy", func() {
			newSpec.SleepInterval = randomDuration()
			Ω(newSpec).NotTo(Equal(orgSpec))
		})
		It("should copy all fields", func() {
			Ω(newSpec).To(Equal(orgSpec))
		})
		It("should return nil when Spec is nil", func() {
			var s *Spec
			Ω(s.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Spec is empty struct", func() {
			var s Spec
			Ω(s.DeepCopy()).To(Equal(&Spec{}))
		})
	})
	When("Validating a Spec", func() {
		It("should return no error when all is default", func() {
			var spec Spec
			Ω(spec.Validate()).To(HaveOccurred())
		})
		It("should return an error when something is not properly set", func() {
			var (
				invalidDuration         = Duration{time.Duration(-1)}
				invalidUInt16           = uint16(0)
				invalidInitMode         = InitMode("invalid")
				initModeNew             = New
				initModeExistingCluster = ExistingCluster
				initModeInvalid         = InitMode("invalid")
				initModePITR            = PITR
				roleReplica             = Replica
				roleInvalid             = Role("invalid")
				suram                   = SUReplAccessMode("invalid")
			)
			for _, spec := range []Spec{
				{SleepInterval: &invalidDuration},
				{RequestTimeout: &invalidDuration},
				{ConvergenceTimeout: &invalidDuration},
				{InitTimeout: &invalidDuration},
				{SyncTimeout: &invalidDuration},
				{DBWaitReadyTimeout: &invalidDuration},
				{FailInterval: &invalidDuration},
				{DeadKeeperRemovalInterval: &invalidDuration},
				{ProxyCheckInterval: &invalidDuration},
				{ProxyTimeout: &invalidDuration},
				{MaxStandbys: &invalidUInt16},
				{MaxStandbysPerSender: &invalidUInt16},
				{MaxSynchronousStandbys: &invalidUInt16},
				{MaxSynchronousStandbys: &invalidUInt16,
					MinSynchronousStandbys: util.ToPtr(uint16(1))},
				{InitMode: nil},
				{AdditionalMasterReplicationSlots: []string{"slot-1"}},
				{AdditionalMasterReplicationSlots: []string{"stolon_slot"}},
				{PGHBA: []string{"local all all md5\n"}},
				{InitMode: &invalidInitMode},
				{InitMode: &initModeNew, Role: &roleReplica},
				{InitMode: &initModeExistingCluster, ExistingConfig: nil},
				{InitMode: &initModeExistingCluster,
					ExistingConfig: &ExistingConfig{KeeperUID: ""}},
				{InitMode: &initModePITR, PITRConfig: nil},
				{InitMode: &initModePITR,
					PITRConfig: &PITRConfig{DataRestoreCommand: ""}},
				{InitMode: &initModePITR,
					PITRConfig: &PITRConfig{RecoveryTargetSettings: &RecoveryTargetSettings{}},
					Role:       &roleReplica},
				{InitMode: &initModeInvalid},
				{DefaultSUReplAccessMode: &suram},
				{Role: &roleReplica, StandbyConfig: nil},
				{Role: &roleInvalid},
			} {
				Ω(spec.Validate()).To(HaveOccurred())
			}
		})
	})
	When("Updating a Spec", func() {
		It("should succeed when all is good", func() {
			var (
				initMode = New
				specRole = Primary
				spec     = Spec{InitMode: &initMode, Role: &specRole}
				cl       = Cluster{Spec: &Spec{InitMode: &initMode, Role: &specRole}}
			)
			Ω(cl.UpdateSpec(&spec)).Error().NotTo(HaveOccurred())
		})
		It("should return an error when the new spec is invalid", func() {
			var (
				initMode = New
				specRole = Primary
				spec     = Spec{InitMode: nil}
				cl       = Cluster{Spec: &Spec{InitMode: &initMode, Role: &specRole}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal("invalid cluster spec: initMode undefined"))
		})
		It("should return an error when initMode changes", func() {
			var (
				specInitMode = PITR
				spec         = Spec{InitMode: &specInitMode,
					PITRConfig: &PITRConfig{DataRestoreCommand: "/bin/true"}}
				clInitMode = New
				cl         = Cluster{Spec: &Spec{InitMode: &clInitMode}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal("cannot change cluster init mode"))
		})
		It("should return an error when Role changes", func() {
			var (
				initMode       = ExistingCluster
				specRole       = Replica
				existingConfig = ExistingConfig{KeeperUID: randomString()}
				standbyConfig  = StandbyConfig{StandbySettings: &StandbySettings{},
					ArchiveRecoverySettings: &ArchiveRecoverySettings{}}
				spec = Spec{
					InitMode:       &initMode,
					Role:           &specRole,
					ExistingConfig: &existingConfig,
					StandbyConfig:  &standbyConfig,
				}
				clRole = Primary
				cl     = Cluster{Spec: &Spec{
					InitMode:       &initMode,
					Role:           &clRole,
					ExistingConfig: &existingConfig,
				}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal(
				"cannot update a cluster from master role to standby role"))
		})
	})
})
