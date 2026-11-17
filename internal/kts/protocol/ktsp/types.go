// SPDX-License-Identifier: Apache-2.0

package ktsp

// The descriptor vocabulary lives in kts/api — describing a service surface is a
// consumer concern, and these shapes are pure data. Re-exported here so code
// holding the protocol package keeps using ktsp.ContainerInfo and friends
// unchanged; the aliases point DOWNWARD at the lighter package.

import ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"

type (
	SlotType         = ktsapi.SlotType
	SlotTags         = ktsapi.SlotTags
	MethodTags       = ktsapi.MethodTags
	CLIArg           = ktsapi.CLIArg
	CLIFlag          = ktsapi.CLIFlag
	CLIMeta          = ktsapi.CLIMeta
	MethodInfo       = ktsapi.MethodInfo
	SlotTypeDep      = ktsapi.SlotTypeDep
	SlotInfo         = ktsapi.SlotInfo
	SlotTypeContract = ktsapi.SlotTypeContract
	ContainerInfo    = ktsapi.ContainerInfo
)

const (
	SlotTypeAgent        = ktsapi.SlotTypeAgent
	SlotTypeSkill        = ktsapi.SlotTypeSkill
	SlotTypeUserPrompter = ktsapi.SlotTypeUserPrompter
	SlotTypeKTSP         = ktsapi.SlotTypeKTSP
)

// Standard tag keys, re-exported alongside the types that carry them.
const (
	TagSlotInternal      = ktsapi.TagSlotInternal
	TagSlotCLI           = ktsapi.TagSlotCLI
	TagSlotCLIFlat       = ktsapi.TagSlotCLIFlat
	TagSlotCLIName       = ktsapi.TagSlotCLIName
	TagSlotCLIAlias      = ktsapi.TagSlotCLIAlias
	TagSlotCLIGroups     = ktsapi.TagSlotCLIGroups
	TagSlotMCP           = ktsapi.TagSlotMCP
	TagSlotMCPPrefix     = ktsapi.TagSlotMCPPrefix
	TagSlotOrdered       = ktsapi.TagSlotOrdered
	TagSlotMaxConcurrent = ktsapi.TagSlotMaxConcurrent

	TagMethodCLI       = ktsapi.TagMethodCLI
	TagMethodCLIName   = ktsapi.TagMethodCLIName
	TagMethodCLIHidden = ktsapi.TagMethodCLIHidden
	TagMethodCLIAlias  = ktsapi.TagMethodCLIAlias
	TagMethodMCP       = ktsapi.TagMethodMCP
	TagMethodMCPName   = ktsapi.TagMethodMCPName
	TagMethodReadOnly  = ktsapi.TagMethodReadOnly
	TagMethodLong      = ktsapi.TagMethodLong
)
