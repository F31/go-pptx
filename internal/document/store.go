package document

import (
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

// PartStore is the minimal editing boundary used by higher-level semantic
// services. It deliberately does not expose a concrete Presentation type.
//
// The interface is introduced before callers are migrated so future modules can
// depend on this smaller contract instead of root-package internals.
type PartStore interface {
	PartBytes(name opc.PartName) ([]byte, error)
	DocumentOf(name opc.PartName) (*xmlstore.XMLDocument, error)
	StagePatch(name opc.PartName, data []byte) error
	StageAdd(name opc.PartName, data []byte, contentType string) error
	StageDelete(name opc.PartName) error
	Commit()
	Revision() uint64
}

// ReadStore is the read-only subset for inspection/planning code.
type ReadStore interface {
	PartBytes(name opc.PartName) ([]byte, error)
	DocumentOf(name opc.PartName) (*xmlstore.XMLDocument, error)
	Revision() uint64
}

// PatchStore is the write subset for applying an already validated edit plan.
type PatchStore interface {
	SinglePatchStore
	StageAdd(name opc.PartName, data []byte, contentType string) error
	StageDelete(name opc.PartName) error
}

// SinglePatchStore is the minimal write boundary for an edit plan that replaces
// exactly one existing part.
type SinglePatchStore interface {
	StagePatch(name opc.PartName, data []byte) error
	Commit()
}
