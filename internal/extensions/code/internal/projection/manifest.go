// SPDX-License-Identifier: Apache-2.0

package projection

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// Coverage is the generated manifest block.
type Coverage struct {
	Source          string                    `json:"source"` // "use-cases/"
	UCTotal         int                       `json:"uc_total"`
	UCByStatus      map[string]int            `json:"uc_by_status"`
	DeferredUCIDs   []string                  `json:"deferred_uc_ids,omitempty"`
	ACTotal         int                       `json:"ac_total"`
	ACByStatus      map[string]int            `json:"ac_by_status"`
	ACByLayer       map[string]int            `json:"ac_by_layer"`
	AntiTotal       int                       `json:"anti_total"`
	CoveragePercent float64                   `json:"coverage_percent"` // AC pass / (total − deferred − skipped)
	BySeverity      map[string]map[string]int `json:"uc_by_severity_status"`
	// Realizes is the functional rollup v0: per referenced feature,
	// AC pass/total counts. Superseded by cm8's transverse verdict at D5.
	Realizes       map[string]*RealizesRollup `json:"realizes_rollup,omitempty"`
	LastVerifiedAt string                     `json:"last_verified_at,omitempty"`
}

// RealizesRollup is the per-US slice of the functional rollup.
type RealizesRollup struct {
	UCs     int `json:"ucs"`
	ACTotal int `json:"ac_total"`
	ACPass  int `json:"ac_pass"`
}

// ComputeCoverage derives the coverage block from the UC files.
func ComputeCoverage(files []*ucfile.UCFile) *Coverage {
	c := &Coverage{
		Source:     "use-cases/",
		UCByStatus: map[string]int{},
		ACByStatus: map[string]int{},
		ACByLayer:  map[string]int{},
		BySeverity: map[string]map[string]int{},
		Realizes:   map[string]*RealizesRollup{},
	}
	pass, denom := 0, 0
	for _, f := range sortedByID(files) {
		m := f.Meta
		ucStatus := orNoTest(m.StatusCache)
		c.UCTotal++
		c.UCByStatus[ucStatus]++
		if ucStatus == ucfile.StatusFailDeferred {
			c.DeferredUCIDs = append(c.DeferredUCIDs, m.ID)
		}
		sev := c.BySeverity[m.Severity]
		if sev == nil {
			sev = map[string]int{}
			c.BySeverity[m.Severity] = sev
		}
		sev[ucStatus]++
		if m.LastVerifiedAt > c.LastVerifiedAt {
			c.LastVerifiedAt = m.LastVerifiedAt
		}

		var rollups []*RealizesRollup
		for _, us := range m.Realizes {
			r := c.Realizes[us]
			if r == nil {
				r = &RealizesRollup{}
				c.Realizes[us] = r
			}
			r.UCs++
			rollups = append(rollups, r)
		}

		for _, ac := range m.AcceptanceCriteria {
			c.ACTotal++
			c.ACByStatus[orNoTest(ac.Status)]++
			c.ACByLayer[ac.Layer]++
			if ac.IsAnti() {
				c.AntiTotal++
			}
			switch ac.Status {
			case ucfile.StatusPass:
				pass++
				denom++
				for _, r := range rollups {
					r.ACTotal++
					r.ACPass++
				}
			case ucfile.StatusFailDeferred, ucfile.StatusSkipped:
				// excluded from the coverage denominator (§11 model)
				for _, r := range rollups {
					r.ACTotal++
				}
			default:
				denom++
				for _, r := range rollups {
					r.ACTotal++
				}
			}
		}
	}
	if len(c.Realizes) == 0 {
		c.Realizes = nil
	}
	if denom > 0 {
		c.CoveragePercent = float64(int(float64(pass)/float64(denom)*1000+0.5)) / 10 // one decimal, stable
	}
	return c
}

// orNoTest reads an absent status as no-test: a use case or criterion no run
// has verified yet is untested, not unknown.
func orNoTest(status string) string {
	if status == "" {
		return ucfile.StatusNoTest
	}
	return status
}
