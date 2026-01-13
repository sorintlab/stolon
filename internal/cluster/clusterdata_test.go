package cluster

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Clusterdata", func() {
	When("Copying a Data", func() {
		var (
			orgData, newData *Data
		)
		BeforeEach(func() {
			orgData = randomData()
			newData = orgData.DeepCopy()
		})
		It("should be a copy", func() {
			newData.FormatVersion = randomUInt64()
			Ω(newData).NotTo(Equal(orgData))
		})
		It("should copy all fields", func() {
			Ω(newData).To(Equal(orgData))
		})
		It("should return nil when Data is nil", func() {
			var d *Data
			Ω(d.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Data is empty struct", func() {
			var d Data
			Ω(d.DeepCopy()).To(Equal(&Data{}))
		})
	})
	When("Defining a new cluster data object", func() {
		It("should return a well defined object", func() {
			c := randomCluster()
			d := NewClusterData(c)
			Ω(d.FormatVersion).To(Equal(CurrentCDFormatVersion))
			Ω(d.Cluster).To(Equal(c))
			Ω(d.Keepers).NotTo(BeNil())
			Ω(d.DBs).NotTo(BeNil())
			Ω(d.Proxy).NotTo(BeNil())
		})
	})
})
