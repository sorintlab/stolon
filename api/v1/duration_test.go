package v1

import (
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Duration", func() {
	When("UnMarshalling", func() {
		It("should work as expected", func() {
			tests := []struct {
				in  string
				d   time.Duration
				err error
			}{
				{in: `"2ms"`, d: time.Millisecond * 2, err: nil},
				{in: `"3s"`, d: time.Second * 3, err: nil},
				{in: `"3h"`, d: time.Hour * 3, err: nil},
				{in: `2ms`, d: 0, err: errors.New("invalid character 'm' after top-level value")},
				{in: `"3 hours"`, d: 0, err: errors.New(`time: unknown unit " hours" in duration "3 hours"`)},
				{in: `"3"`, d: 0, err: errors.New(`time: missing unit in duration "3"`)},
				{in: `3`, d: 0, err: errors.New(`time: missing unit in duration "3"`)},
			}

			for _, tt := range tests {
				dur := &Duration{}
				err := json.Unmarshal([]byte(tt.in), &dur)
				if tt.err != nil {
					Ω(err).To(HaveOccurred())
					Ω(err.Error()).To(Equal(tt.err.Error()))
				} else {
					Ω(err).NotTo(HaveOccurred())
					Ω(dur.Duration).To(Equal(tt.d))
				}
			}
		})
	})
})
