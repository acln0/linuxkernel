// Copyright 2019 Andrei Tudor Călin
//
// Permission to use, copy, modify, and/or distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
// WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
// ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
// WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
// ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
// OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package linuxkernel

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/xerrors"
)

// Symbol represents a kernel symbol.
type Symbol struct {
	Addr   uintptr
	Type   SymbolType
	Name   string
	Module string
}

func (sym Symbol) String() string {
	s := fmt.Sprintf("%016x %c %s", sym.Addr, sym.Type, sym.Name)
	if sym.Module != "" {
		s += fmt.Sprintf(" [%s]", sym.Module)
	}
	return s
}

// SymbolType is the type of a symbol, as reported by nm and /proc/kallsyms.
type SymbolType rune

// Absolute returns a boolean indicating whether the symbol's value is
// absolute, and will not be changed by further linking ('A' or 'a').
func (styp SymbolType) Absolute() bool {
	// TODO(acln): is this correct? What is 'a', exactly?
	return styp == 'A' || styp == 'a'
}

// BSS returns a boolean indicating whether the symbol is in the BSS data
// section ('B' or 'b').
func (styp SymbolType) BSS() bool {
	return styp == 'B' || styp == 'b'
}

// Data returns a boolean indicating whether the symbol is in the initialized
// data section ('D' or 'd').
func (styp SymbolType) Data() bool {
	return styp == 'D' || styp == 'd'
}

// Readonly returns a boolean indicating whether the symbol is in a read only
// data section ('R' or 'r').
func (styp SymbolType) Readonly() bool {
	return styp == 'R' || styp == 'r'
}

// Text returns a boolean indicating whether the symbol is in a text section
// ('T' or 't').
func (styp SymbolType) Text() bool {
	return styp == 'T' || styp == 't'
}

// WeakObject returns a boolean indicating whether the symbol is a weak object
// ('V' or 'v').
func (styp SymbolType) WeakObject() bool {
	return styp == 'V' || styp == 'v'
}

// WeakSymbol returns a boolean indicating whether the symbol is a weak symbol
// ('W' or 'w').
func (styp SymbolType) WeakSymbol() bool {
	return styp == 'W' || styp == 'w'
}

// Global returns a boolean indicating whether the symbol is global (external).
func (styp SymbolType) Global() bool {
	return unicode.IsUpper(rune(styp))
}

// SymbolTable is a Linux kernel symbol table.
type SymbolTable map[Symbol]struct{}

// Find finds symbols with the specified name.
func (symtab SymbolTable) Find(name string) []Symbol {
	var syms []Symbol

	for sym := range symtab {
		if sym.Name == name {
			syms = append(syms, sym)
		}
	}

	return syms
}

func (symtab SymbolTable) parse(line string) error {
	fields := strings.Fields(line)
	if len(fields) != 3 && len(fields) != 4 {
		return xerrors.Errorf("linuxkernel: malformed symbol table line %q", line)
	}

	var sym Symbol

	addr, err := strconv.ParseUint(fields[0], 16, 64)
	if err != nil {
		return xerrors.Errorf("linuxkernel: failed to parse symbol address: %w", err)
	}
	sym.Addr = uintptr(addr)

	symtype := fields[1]
	if len(symtype) != 1 {
		return xerrors.Errorf("linuxkernel: unknown symbol type %q", symtype)
	}
	sym.Type = SymbolType(symtype[0])

	sym.Name = fields[2]

	if len(fields) == 4 {
		sym.Module = strings.TrimFunc(fields[3], func(r rune) bool {
			return r == '[' || r == ']'
		})
	}

	symtab[sym] = struct{}{}
	return nil
}

// Kallsyms calls ParseSymbols("/proc/kallsyms").
func Kallsyms() (SymbolTable, error) {
	return ParseSymbols("/proc/kallsyms")
}

// ParseSymbols reads kernel symbols from the specified path. The path
// should indicate /proc/kallsyms or the equivalent file if procfs is
// mounted elsewhere.
func ParseSymbols(path string) (SymbolTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	symtab := make(SymbolTable)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if err := symtab.parse(sc.Text()); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	return symtab, nil
}
