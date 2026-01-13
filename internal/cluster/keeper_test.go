package cluster

import (
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Keeper", func() {
	When("Copying a Data", func() {
		var (
			orgKIs, newKIs KeepersInfo
		)
		BeforeEach(func() {
			orgKIs = randomKeepersInfo(3)
			newKIs = orgKIs.DeepCopy()
		})
		It("should be a copy", func() {
			for key := range newKIs {
				newKIs[key].BootUUID = randomString()
			}
			Ω(newKIs).NotTo(Equal(orgKIs))
		})
		It("should copy all fields", func() {
			Ω(newKIs).To(Equal(orgKIs))
		})
		It("should return empty struct when KeepersInfo is empty struct", func() {
			var kis KeepersInfo
			Ω(kis.DeepCopy()).To(BeNil())
		})
	})
	When("Copying a KeeperInfo", func() {
		var (
			orgKS, newKS *KeeperInfo
		)
		BeforeEach(func() {
			orgKS = randomKeeperInfo()
			newKS = orgKS.DeepCopy()
		})
		It("should be a copy", func() {
			newKS.BootUUID = randomString()
			Ω(newKS).NotTo(Equal(orgKS))
		})
		It("should copy all fields", func() {
			Ω(newKS).To(Equal(orgKS))
		})
		It("should return nil when KeeperInfo is nil", func() {
			var ki *KeeperInfo
			Ω(ki.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when KeeperInfo is empty struct", func() {
			var ki KeeperInfo
			Ω(ki.DeepCopy()).To(Equal(&KeeperInfo{}))
		})
	})
	When("Copying a ProxiesInfo", func() {
		var (
			orgPIs, newPIs ProxiesInfo
		)
		BeforeEach(func() {
			orgPIs = randomProxiesInfo(5)
			newPIs = orgPIs.DeepCopy()
		})
		It("should be a copy", func() {
			for key := range newPIs {
				newPIs[key].Generation = randomInt64()
			}
			Ω(newPIs).NotTo(Equal(orgPIs))
		})
		It("should copy all fields", func() {
			Ω(newPIs).To(Equal(orgPIs))
		})
		It("should return nil when ProxiesInfo is nil", func() {
			var pis ProxiesInfo
			Ω(pis.DeepCopy()).To(BeNil())
		})
	})
	// NewKeeperFromKeeperInfo
	When("Creating a nw KeeperInfo", func() {
		var (
			ki *KeeperInfo
		)
		BeforeEach(func() {
			ki = randomKeeperInfo()
		})
		It("should be a copy", func() {
			k := NewKeeperFromKeeperInfo(ki)
			Ω(k.UID).To(Equal(ki.UID))
			Ω(k.Generation).To(Equal(InitialGeneration))
			Ω(k.ChangeTime.IsZero()).To(BeTrue())
			Ω(k.Spec).To(Equal(&KeeperSpec{}))
			Ω(k.Status.Healthy).To(BeTrue())
			Ω(k.Status.LastHealthyTime.IsZero()).NotTo(BeTrue())
			Ω(k.Status.BootUUID).To(Equal(ki.BootUUID))
		})
	})
	// Keepers.SortedKeys
	When("Requesting keys of a Keepers object", func() {
		It("should returnn sorted keys list", func() {
			k := randomKeepers(10)
			var keys []string
			for key := range k {
				keys = append(keys, key)
			}
			Ω(slices.IsSorted(keys)).NotTo(BeTrue())
			sortedKeys := k.SortedKeys()
			Ω(slices.IsSorted(sortedKeys)).To(BeTrue())
			Ω(slices.IsSorted(sortedKeys)).NotTo(Equal(keys))
			slices.Sort(keys)
			Ω(slices.IsSorted(keys)).To(BeTrue())
			Ω(sortedKeys).To(Equal(keys))
		})
	})
})
