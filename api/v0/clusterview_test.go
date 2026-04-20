package v0

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Clusterview", func() {
	When("Copying a KeepersState", func() {
		var (
			orgKss, newKss KeepersState
		)
		BeforeEach(func() {
			orgKss = randomKeepersState(5)
			newKss = orgKss.Copy()
		})
		It("should be a copy", func() {
			for key := range newKss {
				newKss[key].ClusterViewVersion = randomInt()
			}
			Ω(newKss).NotTo(Equal(orgKss))
		})
		It("should copy all fields", func() {
			Ω(newKss).To(Equal(orgKss))
		})
		It("should return nil when KeepersState is nil", func() {
			var kss KeepersState
			Ω(kss.Copy()).To(BeNil())
		})
	})
	When("Copying a KeeperState", func() {
		var (
			orgKS, newKS *KeeperState
		)
		BeforeEach(func() {
			orgKS = randomKeeperState()
			newKS = orgKS.Copy()
		})
		It("should be a copy", func() {
			newKS.ClusterViewVersion = randomInt()
			Ω(newKS).NotTo(Equal(orgKS))
		})
		It("should copy all fields", func() {
			Ω(newKS).To(Equal(orgKS))
		})
		It("should return nil when KeeperState is nil", func() {
			var ki *KeeperState
			Ω(ki.Copy()).To(BeNil())
		})
		It("should return empty struct when KeeperState is empty struct", func() {
			var ki KeeperState
			Ω(ki.Copy()).To(Equal(&KeeperState{}))
		})
	})
	When("Copying a KeepersRole", func() {
		var (
			orgKsr, newKsr KeepersRole
		)
		BeforeEach(func() {
			orgKsr = randomKeepersRole(5)
			newKsr = orgKsr.Copy()
		})
		It("should be a copy", func() {
			for key := range newKsr {
				newKsr[key].Follow = randomString()
			}
			Ω(newKsr).NotTo(Equal(orgKsr))
		})
		It("should copy all fields", func() {
			Ω(newKsr).To(Equal(orgKsr))
		})
		It("should return nil when KeepersRole is nil", func() {
			var kss KeepersRole
			Ω(kss.Copy()).To(BeNil())
		})
	})
	When("Copying a KeeperRole", func() {
		var (
			orgKR, newKR *KeeperRole
		)
		BeforeEach(func() {
			orgKR = randomKeeperRole()
			newKR = orgKR.Copy()
		})
		It("should be a copy", func() {
			newKR.Follow = randomString()
			Ω(newKR).NotTo(Equal(orgKR))
		})
		It("should copy all fields", func() {
			Ω(newKR).To(Equal(orgKR))
		})
		It("should return nil when KeeperState is nil", func() {
			var ki *KeeperRole
			Ω(ki.Copy()).To(BeNil())
		})
		It("should return empty struct when KeeperState is empty struct", func() {
			var ki KeeperRole
			Ω(ki.Copy()).To(Equal(&KeeperRole{}))
		})
	})
	When("Copying a ProxyConf", func() {
		var (
			orgPC, newPC *ProxyConf
		)
		BeforeEach(func() {
			orgPC = randomProxyConf()
			newPC = orgPC.Copy()
		})
		It("should be a copy", func() {
			newPC.Port = randomPort()
			Ω(newPC).NotTo(Equal(orgPC))
		})
		It("should copy all fields", func() {
			Ω(newPC).To(Equal(orgPC))
		})
		It("should return nil when ProxyConf is nil", func() {
			var ki *ProxyConf
			Ω(ki.Copy()).To(BeNil())
		})
		It("should return empty struct when ProxyConf is empty struct", func() {
			var ki ProxyConf
			Ω(ki.Copy()).To(Equal(&ProxyConf{}))
		})
	})
	When("Copying a ProxyConf", func() {
		var (
			orgCV, newCV *ClusterView
		)
		BeforeEach(func() {
			orgCV = randomClusterView()
			newCV = orgCV.Copy()
		})
		It("should be a copy", func() {
			newCV.Master = randomString()
			Ω(newCV).NotTo(Equal(orgCV))
			Ω(newCV.Equals(orgCV)).NotTo(BeTrue())
		})
		It("should copy all fields", func() {
			Ω(newCV).To(Equal(orgCV))
			Ω(newCV.Equals(orgCV)).To(BeTrue())
		})
		It("should return nil when ClusterView is nil", func() {
			var ki *ClusterView
			Ω(ki.Copy()).To(BeNil())
		})
		It("should return empty struct when ClusterView is empty struct", func() {
			var ki ClusterView
			Ω(ki.Copy()).To(Equal(&ClusterView{}))
		})
	})
	When("Requesting sorted keys from KeepersState", func() {
		var (
			orgKss     KeepersState
			sortedKeys []string
		)
		BeforeEach(func() {
			orgKss = randomKeepersState(20)
			sortedKeys = orgKss.SortedKeys()
		})
		It("should have same length as keepersState", func() {
			Ω(sortedKeys).To(HaveLen(len(orgKss)))
		})
		It("should contain all keys", func() {
			for key := range orgKss {
				Ω(sortedKeys).To(ContainElement(key))
			}
		})
		It("should be sorted", func() {
			sortedList := make([]string, len(sortedKeys))
			copy(sortedList, sortedKeys)
			sort.Strings(sortedList)
			Ω(sortedKeys).To(Equal(sortedList))
		})
	})
	When("Adding new KeeperState to KeepersState from KeeperInfo", Ordered, func() {
		var (
			newInfo = randomKeeperInfo()
			states  = randomKeepersState(5)
		)
		It("should properly generate and add a KeeperState if it is not in yet", func() {
			Ω(states.NewFromKeeperInfo(newInfo)).NotTo(HaveOccurred())
		})
		It("should raise error if info is added a second time", func() {
			Ω(states.NewFromKeeperInfo(newInfo)).To(HaveOccurred())
		})
	})
	When("Checking if a KeeperState has changed", func() {
		var (
			i KeeperInfo
			s KeeperState
		)
		BeforeEach(func() {
			i = *randomKeeperInfo()
			s = KeeperState{ID: i.ID}
			Ω(s.UpdateFromKeeperInfo(&i)).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			Ω(s.UpdateFromKeeperInfo(&i)).NotTo(HaveOccurred())
			Ω(s.ChangedFromKeeperInfo(&i)).NotTo(BeTrue())
		})
		It("should return true when ClusterViewVersion is changed", func() {
			s.ClusterViewVersion = randomInt()
			Ω(s.ChangedFromKeeperInfo(&i)).To(BeTrue())
		})
		It("should return true when ListenAddress is changed", func() {
			s.ListenAddress = randomIP()
			Ω(s.ChangedFromKeeperInfo(&i)).To(BeTrue())
		})
		It("should return true when Port is changed", func() {
			s.Port = randomPort()
			Ω(s.ChangedFromKeeperInfo(&i)).To(BeTrue())
		})
		It("should return true when PGListenAddress is changed", func() {
			s.PGListenAddress = randomIP()
			Ω(s.ChangedFromKeeperInfo(&i)).To(BeTrue())
		})
		It("should return true when PGPort is changed", func() {
			s.PGPort = randomPort()
			Ω(s.ChangedFromKeeperInfo(&i)).To(BeTrue())
		})
	})
	When("Working with KeeperState with errors", func() {
		var (
			s KeeperState
		)
		BeforeEach(func() {
			s = *randomKeeperState()
		})
		It("should properly set error", func() {
			s.SetError()
			Ω(s.ErrorStartTime.IsZero()).To(BeFalse())
		})
		It("should properly reset error", func() {
			s.CleanError()
			Ω(s.ErrorStartTime.IsZero()).To(BeTrue())
		})
	})
	When("Working with KeepersRole", func() {
		var (
			r KeepersRole
		)
		BeforeEach(func() {
			r = NewKeepersRole()
		})
		It("should be empty on initialization", func() {
			Ω(r).To(HaveLen(0))
		})
		It("should be empty on initialization", func() {
			var (
				id     = randomString()
				follow = randomString()
				newKR  = &KeeperRole{ID: id, Follow: follow}
			)
			r.Add(id, follow)
			Ω(r).To(HaveLen(1))
			Ω(r).To(HaveKeyWithValue(id, newKR))
		})
	})
	When("Working with a ClusterView", func() {
		It("should successfully parse", func() {
		})
	})
	/*
		NewClusterView()
		ClusterView.Equals()
	*/

	When("Creating getting followers ids from a ClusterView", func() {
		var (
			v ClusterView
		)
		BeforeEach(func() {
			v = *randomClusterView()
		})
		It("should properly work as expected", func() {
			for _, role := range v.KeepersRole {
				fIDs := v.GetFollowersIDs(role.Follow)
				Ω(fIDs).To(HaveLen(1))
			}
		})
	})
})
