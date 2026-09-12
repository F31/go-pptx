package editplan

import (
	"github.com/F31/go-pptx/internal/document"
	"github.com/F31/go-pptx/internal/opc"
)

// SinglePartPatch is the first, deliberately narrow edit plan: replace the
// complete bytes of one already existing part, then commit the transaction.
//
// It is intentionally not a multi-part plan yet. Multi-part apply needs rollback
// semantics before it can be a safe shared abstraction.
type SinglePartPatch struct {
	part opc.PartName
	data []byte
}

// NewSinglePartPatch creates a single-part edit plan. The payload is copied so
// callers can reuse or mutate their source buffer after planning.
func NewSinglePartPatch(part opc.PartName, data []byte) SinglePartPatch {
	return SinglePartPatch{part: part, data: append([]byte(nil), data...)}
}

// Part returns the target OPC part name.
func (p SinglePartPatch) Part() opc.PartName { return p.part }

// Data returns a defensive copy of the replacement bytes.
func (p SinglePartPatch) Data() []byte { return append([]byte(nil), p.data...) }

// Apply stages and commits the plan.
func (p SinglePartPatch) Apply(store document.SinglePatchStore) error {
	if err := store.StagePatch(p.part, p.data); err != nil {
		return err
	}
	store.Commit()
	return nil
}
