// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// : the form steps load only whole - a start_task names its
// task, an answer_form its value.
func TestTheFormStepsAreCheckedAtLoad(t *testing.T) {
	for _, c := range []struct {
		step Step
		ok   bool
	}{
		{Step{StartTask: &StartTaskStep{Task: "t"}}, true},
		{Step{StartTask: &StartTaskStep{}}, false},
		{Step{AnswerForm: &AnswerFormStep{Value: map[string]any{"a": 1}}}, true},
		{Step{AnswerForm: &AnswerFormStep{}}, false},
		{Step{StartTask: &StartTaskStep{Task: "t"}, AnswerForm: &AnswerFormStep{Value: 1}}, false},
	} {
		if err := checkSteps([]Step{c.step}, "stage s"); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.step, err)
		}
	}
}
