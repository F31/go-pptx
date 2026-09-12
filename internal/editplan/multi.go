package editplan

import (
	"github.com/F31/go-pptx/internal/document"
	"github.com/F31/go-pptx/internal/opc"
)

// OpKind identifies a staged document operation.
type OpKind int

const (
	OpPatch OpKind = iota
	OpAdd
	OpDelete
)

// Operation is one staged change in a multi-part edit plan.
type Operation struct {
	kind        OpKind
	part        opc.PartName
	data        []byte
	contentType string
}

func Patch(part opc.PartName, data []byte) Operation {
	return Operation{kind: OpPatch, part: part, data: append([]byte(nil), data...)}
}

func Add(part opc.PartName, data []byte, contentType string) Operation {
	return Operation{kind: OpAdd, part: part, data: append([]byte(nil), data...), contentType: contentType}
}

func Delete(part opc.PartName) Operation {
	return Operation{kind: OpDelete, part: part}
}

func (o Operation) Kind() OpKind { return o.kind }

func (o Operation) Part() opc.PartName { return o.part }

func (o Operation) Data() []byte { return append([]byte(nil), o.data...) }

func (o Operation) ContentType() string { return o.contentType }

// MultiPartPlan stages multiple operations as one transaction. It only stages;
// callers that own the concrete document must provide rollback and commit.
type MultiPartPlan struct {
	ops []Operation
}

func NewMultiPartPlan(ops ...Operation) MultiPartPlan {
	out := MultiPartPlan{ops: make([]Operation, len(ops))}
	for i, op := range ops {
		out.ops[i] = cloneOperation(op)
	}
	return out
}

func (p MultiPartPlan) Operations() []Operation {
	out := make([]Operation, len(p.ops))
	for i, op := range p.ops {
		out[i] = cloneOperation(op)
	}
	return out
}

func (p MultiPartPlan) Stage(store document.PatchStore) error {
	for _, op := range p.ops {
		switch op.kind {
		case OpPatch:
			if err := store.StagePatch(op.part, op.data); err != nil {
				return err
			}
		case OpAdd:
			if err := store.StageAdd(op.part, op.data, op.contentType); err != nil {
				return err
			}
		case OpDelete:
			if err := store.StageDelete(op.part); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneOperation(op Operation) Operation {
	op.data = append([]byte(nil), op.data...)
	return op
}
