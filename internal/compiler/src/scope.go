package src

import (
	"errors"
	"fmt"
	"strings"

	ts "github.com/nevalang/neva/pkg/typesystem"
)

// Scope is an entity that can be used to query the program.
type Scope struct {
	Loc  ScopeLocation // It keeps track of current location to properly resolve imports and local references.
	Prog Program       // And of course it does has access to the program itself.
}

// ScopeLocation is used by scope to resolve references.
type ScopeLocation struct {
	PkgName  string // Full (real) name of the current package
	FileName string // Name of the current file in the current package
}

// IsTopType returns true if passed reference is builtin "any" and false otherwise.
func (s Scope) IsTopType(expr ts.Expr) bool {
	if expr.Inst == nil {
		return false
	}
	parsed, ok := expr.Inst.Ref.(EntityRef)
	if !ok {
		return false
	}
	if parsed.Name != "any" {
		return false
	}
	switch parsed.Pkg {
	case "builtin", "":
		return true
	}
	return parsed.Pkg == "" || parsed.Pkg == "builtin"
}

// Get type takes reference to type and returns def and scope with updated location if type found, or error.
func (s Scope) GetType(ref fmt.Stringer) (ts.Def, ts.Scope, error) {
	parsedRef, ok := ref.(EntityRef)
	if !ok {
		return ts.Def{}, Scope{}, fmt.Errorf("ref is not entity ref: %v", ref)
	}

	def, location, err := s.getType(parsedRef)
	if err != nil {
		return ts.Def{}, Scope{}, fmt.Errorf("get type: %w", err)
	}

	if parsedRef.Pkg == "" {
		return def, s, nil
	}

	return def, Scope{
		Loc:  location,
		Prog: s.Prog,
	}, nil
}

// parseRef assumes string-ref has form of <pkg_name>.<entity_name≥ or just <entity_name>.
func (s Scope) parseRef(ref string) EntityRef {
	var entityRef EntityRef

	parts := strings.Split(ref, ".")
	if len(parts) == 2 {
		entityRef.Pkg = parts[0]
		entityRef.Name = parts[1]
	} else {
		entityRef.Name = ref
	}

	return entityRef
}

var ErrEntityNotType = errors.New("entity not type")

func (s Scope) getType(ref EntityRef) (ts.Def, ScopeLocation, error) {
	entity, found, err := s.Entity(ref)
	if err != nil {
		return ts.Def{}, ScopeLocation{}, err
	}

	if entity.Kind != TypeEntity {
		return ts.Def{}, ScopeLocation{}, fmt.Errorf("%w: want %v, got %v", ErrEntityNotType, TypeEntity, entity.Kind)
	}

	return entity.Type, found, nil
}

var (
	ErrNoImport          = errors.New("no import found")
	ErrEntityNotExported = errors.New("entity is not exported")
)

// Entity returns entity by passed reference.
// If entity is local (ref has no pkg) the current location.pkg is used
// Otherwise we use current file imports to resolve external ref.
func (s Scope) Entity(entityRef EntityRef) (Entity, ScopeLocation, error) {
	if entityRef.Pkg == "" {
		entity, filename, ok := s.Prog[s.Loc.PkgName].Entity(entityRef.Name)
		if ok {
			return entity, ScopeLocation{
				PkgName:  s.Loc.PkgName,
				FileName: filename,
			}, nil
		}

		entity, filename, ok = s.Prog["std/builtin"].Entity(entityRef.Name)
		if !ok {
			return Entity{}, ScopeLocation{}, fmt.Errorf("%w: %v", ErrEntityNotFound, entityRef)
		}

		return entity, ScopeLocation{
			PkgName:  "std/builtin",
			FileName: filename,
		}, nil
	}

	realImportPkgName, ok := s.Prog[s.Loc.PkgName][s.Loc.FileName].Imports[entityRef.Pkg]
	if !ok {
		return Entity{}, ScopeLocation{}, fmt.Errorf("%w: %v", ErrNoImport, entityRef.Pkg)
	}

	entity, fileName, err := s.Prog.Entity(EntityRef{
		Pkg:  realImportPkgName,
		Name: entityRef.Name,
	})
	if err != nil {
		return Entity{}, ScopeLocation{}, fmt.Errorf("prog entity: %w", err)
	}

	if !entity.Exported {
		return Entity{}, ScopeLocation{}, fmt.Errorf("%w: %v", ErrEntityNotExported, entityRef.Name)
	}

	return entity, ScopeLocation{
		PkgName:  realImportPkgName,
		FileName: fileName,
	}, nil
}