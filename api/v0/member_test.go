package v0

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/stolon/internal/common"
)

var _ = Describe("Member", func() {
	When("Copying a KeeperInfo", func() {
		var (
			orgKI, newKI *KeeperInfo
		)
		BeforeEach(func() {
			orgKI = randomKeeperInfo()
			newKI = orgKI.Copy()
		})
		It("should be a copy", func() {
			newKI.ClusterViewVersion = randomInt()
			Ω(newKI).NotTo(Equal(orgKI))
		})
		It("should copy all fields", func() {
			Ω(newKI).To(Equal(orgKI))
		})
		It("should return nil when KeeperInfo is nil", func() {
			var ki *KeeperInfo
			Ω(ki.Copy()).To(BeNil())
		})
		It("should return empty struct when KeeperInfo is empty struct", func() {
			var ki KeeperInfo
			Ω(ki.Copy()).To(Equal(&KeeperInfo{}))
		})
	})
	When("Copying a PostgresTimelinesHistory", func() {
		var (
			orgPTH, newPTH PostgresTimelinesHistory
		)
		BeforeEach(func() {
			orgPTH = randomTLSH()
			newPTH = orgPTH.Copy()
		})
		It("should be a copy", func() {
			newPTH[0].Reason = randomString()
			Ω(newPTH).NotTo(Equal(orgPTH))
		})
		It("should copy all fields", func() {
			Ω(newPTH).To(Equal(orgPTH))
		})
		It("should return nil when PostgresTimelinesHistory is nil", func() {
			var pth *PostgresTimelinesHistory
			Ω(pth.Copy()).To(BeNil())
		})
		It("should return nil when PostgresTimelinesHistory is nil list", func() {
			var pth PostgresTimelinesHistory
			Ω(pth.Copy()).To(BeNil())
		})
	})
	When("Copying a PostgresState", func() {
		var (
			orgPS, newPS *PostgresState
		)
		BeforeEach(func() {
			orgPS = &PostgresState{
				Initialized:      randomBool(),
				Role:             common.Role(randomString()),
				SystemID:         randomString(),
				TimelineID:       randomUInt64(),
				XLogPos:          randomUInt64(),
				TimelinesHistory: randomTLSH(),
			}
			newPS = orgPS.Copy()
		})
		It("should be a copy", func() {
			newPS.TimelineID = randomUInt64()
			Ω(newPS).NotTo(Equal(orgPS))
		})
		It("should copy all fields", func() {
			Ω(newPS).To(Equal(orgPS))
		})
		It("should return nil when PostgresState is nil", func() {
			var ps *PostgresState
			Ω(ps.Copy()).To(BeNil())
		})
		It("should return nil when PostgresState is nil list", func() {
			var ps PostgresState
			Ω(ps.Copy()).To(Equal(&PostgresState{}))
		})
	})
	When("Geting a timeline history", func() {
		var (
			tlid        = randomUInt64()
			switchPoint = randomUInt64()
			reason      = randomString
			orgPgTlH    = PostgresTimelineHistory{
				TimelineID:  tlid,
				SwitchPoint: switchPoint,
				Reason:      reason(),
			}
			pgtlhs = PostgresTimelinesHistory{
				randomTLH(),
				randomTLH(),
				&orgPgTlH,
				randomTLH(),
				randomTLH(),
				randomTLH(),
			}
		)
		It("should work as expected", func() {
			fetchedTLH := pgtlhs.GetTimelineHistory(tlid)
			Ω(*fetchedTLH).To(Equal(orgPgTlH))
		})
	})
})
