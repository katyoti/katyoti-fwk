// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"github.com/katyoti/katyoti-fwk/internal/core/el"
)

// filterExprs compiles a task's `filter:` against the object vocabulary
// (el.ObjectVocabulary) at parse time, so a filter that names anything else —
// the retired `node.*` vocabulary above all — is refused naming the task,
// instead of loading and then disabling its action forever.
//
// The filter is EVALUATED by CWM, in the object action list, against the same
// vocabulary. There is no second evaluator here any more: `MatchesNode` bound
// `{node: …}`, had no production caller, and was the one place the same
// `filter:` meant something different (p7o, 2026-09-25).
var filterExprs = el.NewEnvCache(el.ObjectVocabulary())
