# gocode-acceptance — throwaway target for ktw-gocode v1 acceptance

This is the **purpose-built fresh test module** described in
[modules/code-gocode/concepts.md §1](../../../modules/code-gocode/concepts.md).

**What it is**: a deliberately minimal Go module that follows the
3-package convention (`public/internal/local`) and exposes 1-2
public functions with a small test suite. Used by ktw-gocode's
acceptance test to prove the full loop:

1. **Greenfield mode**: ktw-gocode autonomously bootstraps a fresh
   charter / UCs / tests / audit / tier-1 promotion on this module
2. **Retro-engineering mode**: after resetting state, `module.discover`
   re-derives the items from the existing Go code

**What it is NOT**:

- Production code — nothing imports this module, nothing depends on
  it, it ships only as test fixture
- A library you should extend — it stays minimal forever to keep the
  acceptance test fast and predictable
- Representative of project conventions — `internal/extensions/` is
  where real modules live; this directory exists only for the
  acceptance fixture

## Structure (mirrors the three-package layout, modules/code-gocode/concepts.md §2)

```
gocode-acceptance/
├── README.md           # this file
├── module.yaml         # starter manifest declaring kind=library
├── public/             # exported API surface
│   ├── calc.go         # Add + Subtract functions
│   └── calc_test.go    # L1 tests
├── internal/           # private implementation
│   ├── checker.go      # input validation helper
│   └── checker_test.go # L1 tests
└── local/              # framework contribution stub
    └── extension.go    # empty application.Extension factory
```

## Rules this module is supposed to satisfy

- ✅ 3-package layout (public / internal / local)
- ✅ public/ exports have godoc
- ✅ internal/ never imported from another module
- ✅ At least one L1 test covering the happy path + edge case
- ✅ kind: library declared in module.yaml
- ⏸ UCs (not yet authored — that's what specifier persona does)
- ⏸ Charter (not yet authored — that's what architect persona does)
- ⏸ L2/L3 tests (not yet authored — tester adds them)

The unchecked items are the **point of the acceptance test**:
ktw-gocode running autonomously turns them into checked items.
