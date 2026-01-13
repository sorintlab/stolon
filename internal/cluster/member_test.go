package cluster

import (
	"slices"
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Member", func() {
	When("Copying a PostgresState", func() {
		var (
			orgPS, newPS *PostgresState
		)
		BeforeEach(func() {
			orgPS = randomPostgresState()
			newPS = orgPS.DeepCopy()
		})
		It("should be a copy", func() {
			newPS.Generation = randomInt64()
			Ω(newPS).NotTo(Equal(orgPS))
		})
		It("should copy all fields", func() {
			Ω(newPS).To(Equal(orgPS))
		})
		It("should return nil when PostgresState is nil", func() {
			var ps *PostgresState
			Ω(ps.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when PostgresState is empty struct", func() {
			var ps PostgresState
			Ω(ps.DeepCopy()).To(Equal(&PostgresState{}))
		})
	})
	When("finding a timeline history", func() {
		It("should be a copy", func() {
			pths := randomTLSH(5)
			for _, pth := range pths {
				Ω(pths.GetTimelineHistory(pth.TimelineID)).To(Equal(pth))
			}
		})
		It("should return nil when requesting an unknown timeline", func() {
			Ω(randomTLSH(5).GetTimelineHistory(randomUInt64())).To(BeNil())
		})
	})
	When("Converting a ProxiesInfo to a slice", func() {
		It("should work as expected", func() {
			psi := randomProxiesInfo(5)
			psiList := psi.ToSlice()
			Ω(psiList).To(HaveLen(len(psi)))
			for _, value := range psi {
				Ω(psiList).To(ContainElement(value))
			}
		})
	})
	When("sorting a SentinelsInfo", func() {
		ssi := randomSentinelsInfo(5)
		It("should not be sorted when generated at random", func() {
			var keys []string
			for _, si := range ssi {
				keys = append(keys, si.UID)
			}
			Ω(slices.IsSorted(keys)).NotTo(BeTrue())
		})
		It("should be sorted after doing the sort operation", func() {
			sort.Sort(ssi)
			var keys []string
			for _, si := range ssi {
				keys = append(keys, si.UID)
			}
			Ω(slices.IsSorted(keys)).To(BeTrue())
		})
	})
	When("sorting a SentinelsInfo", func() {
		psi := randomProxiesInfo(5).ToSlice()
		It("should not be sorted when generated at random", func() {
			var keys []string
			for _, si := range psi {
				keys = append(keys, si.UID)
			}
			Ω(slices.IsSorted(keys)).NotTo(BeTrue())
		})
		It("should be sorted after doing the sort operation", func() {
			sort.Sort(psi)
			var keys []string
			for _, si := range psi {
				keys = append(keys, si.UID)
			}
			Ω(slices.IsSorted(keys)).To(BeTrue())
		})
	})
})
