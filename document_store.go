package pptx

import (
	"github.com/F31/go-pptx/internal/document"
	"github.com/F31/go-pptx/internal/editplan"
	"github.com/F31/go-pptx/internal/opc"
	"github.com/F31/go-pptx/internal/xmlstore"
)

var _ document.PartStore = presentationPartStore{}

// presentationPartStore adapts Presentation internals to internal/document
// without adding exported methods to Presentation's public API.
type presentationPartStore struct {
	p *Presentation
}

func (p *Presentation) documentStore() document.PartStore {
	return presentationPartStore{p: p}
}

func (s presentationPartStore) PartBytes(name opc.PartName) ([]byte, error) {
	return s.p.partBytes(name)
}

func (s presentationPartStore) DocumentOf(name opc.PartName) (*xmlstore.XMLDocument, error) {
	return s.p.docOf(name)
}

func (s presentationPartStore) StagePatch(name opc.PartName, data []byte) error {
	return s.p.stagePatch(name, data)
}

func (s presentationPartStore) StageAdd(name opc.PartName, data []byte, contentType string) error {
	return s.p.stageAdd(name, data, contentType)
}

func (s presentationPartStore) StageDelete(name opc.PartName) error {
	return s.p.stageDelete(name)
}

func (s presentationPartStore) Commit() {
	s.p.commit()
}

func (s presentationPartStore) Revision() uint64 {
	return s.p.Revision()
}

func applySinglePartPatch(p *Presentation, part opc.PartName, data []byte) error {
	plan := editplan.NewSinglePartPatch(part, data)
	return plan.Apply(p.documentStore())
}

func applyMultiPartPlan(p *Presentation, plan editplan.MultiPartPlan) error {
	before := cloneChangeSet(p.pending)
	if err := plan.Stage(p.documentStore()); err != nil {
		p.pending = before
		return err
	}
	p.commit()
	return nil
}

func relsPlanOp(p *Presentation, part opc.PartName, content []byte) editplan.Operation {
	rp := relsPart(part)
	if p.pk.HasPart(rp) || p.addedParts[rp].Content != nil {
		return editplan.Patch(rp, content)
	}
	return editplan.Add(rp, content, "")
}

func cloneChangeSet(cs *opc.ChangeSet) *opc.ChangeSet {
	if cs == nil {
		return nil
	}
	out := &opc.ChangeSet{}
	if len(cs.Patched) > 0 {
		out.Patched = make(map[opc.PartName][]byte, len(cs.Patched))
		for name, data := range cs.Patched {
			out.Patched[name] = append([]byte(nil), data...)
		}
	}
	if len(cs.Added) > 0 {
		out.Added = make(map[opc.PartName]opc.AddedPart, len(cs.Added))
		for name, added := range cs.Added {
			added.Content = append([]byte(nil), added.Content...)
			out.Added[name] = added
		}
	}
	if len(cs.Deleted) > 0 {
		out.Deleted = make(map[opc.PartName]bool, len(cs.Deleted))
		for name, deleted := range cs.Deleted {
			out.Deleted[name] = deleted
		}
	}
	return out
}
