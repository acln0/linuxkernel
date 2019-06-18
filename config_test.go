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
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestConfig(t *testing.T) {
	t.Run("Parse", testConfigParse)
	t.Run("Equal", testConfigEqual)
	t.Run("WriteTo", testConfigWriteTo)
	t.Run("WriteToPredictableOrder", testConfigWriteToPredictableOrder)
	t.Run("ApplyDiff", testApplyDiff)
}

func TestDiffConfig(t *testing.T) {
	t.Run("Basic", testDiffConfigBasic)
	t.Run("Symmetry", testDiffConfigSymmetry)
}

func TestConfigDiff(t *testing.T) {
	t.Run("WriteTo", testConfigDiffWriteTo)
	t.Run("WriteToPredictableOrder", testConfigDiffWriteToPredictableOrder)
}

func testConfigParse(t *testing.T) {
	input := "# comment\n# CONFIG_X is not set\nCONFIG_Y=y\nCONFIG_Z=\"\"\n"
	got, err := ParseConfig(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{"X": "n", "Y": "y", "Z": `""`}
	if !got.Equal(want) {
		t.Fatalf("ParseConfig(%q): got %#v, want %#v", input, got, want)
	}
}

func testConfigEqual(t *testing.T) {
	cfg := Config{"X": "n", "Y": "y", "Z": `""`}
	if !cfg.Equal(cfg) {
		t.Fatalf("%#v is not equal to itself", cfg)
	}
	others := []Config{
		{"X": "n", "Y": "y", "Z": `""`, "T": "42"},
		{"X": "", "Y": "y", "Z": `""`},
		{"Y": "y", "Z": `""`},
	}
	for _, other := range others {
		if cfg.Equal(other) {
			t.Fatalf("%#v equal to %#v", cfg, other)
		}
	}
}

func testConfigWriteTo(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cfg := Config{
			"FOO":  "n",
			"BAR":  "y",
			"BAZ":  `""`,
			"QUUX": "42",
		}
		wantsb := new(strings.Builder)
		wantsb.WriteString(`CONFIG_BAR=y` + "\n")
		wantsb.WriteString(`CONFIG_BAZ=""` + "\n")
		wantsb.WriteString(`# CONFIG_FOO is not set` + "\n")
		wantsb.WriteString(`CONFIG_QUUX=42` + "\n")
		if _, err := cfg.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), wantsb.String(); got != want {
			t.Fatalf("%#v.WriteTo() => %q, want %q", cfg, got, want)
		}
	})
	t.Run("Nil", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatal("WriteTo crashed on nil Config")
			}
		}()
		buf := new(bytes.Buffer)
		var cfg Config
		if _, err := cfg.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
		if len(buf.Bytes()) != 0 {
			t.Fatalf("nil Config.WriteTo produced output")
		}
	})
	t.Run("Empty", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatal("WriteTo crashed on empty Config")
			}
		}()
		buf := new(bytes.Buffer)
		cfg := make(Config)
		if _, err := cfg.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
		if len(buf.Bytes()) != 0 {
			t.Fatalf("empty Config.WriteTo produced output")
		}
	})
}

func testConfigWriteToPredictableOrder(t *testing.T) {
	cfg := Config{
		"FOO":  "n",
		"BAR":  "y",
		"BAZ":  `""`,
		"QUUX": "42",
	}
	wantsb := new(strings.Builder)
	wantsb.WriteString(`CONFIG_BAR=y` + "\n")
	wantsb.WriteString(`CONFIG_BAZ=""` + "\n")
	wantsb.WriteString(`# CONFIG_FOO is not set` + "\n")
	wantsb.WriteString(`CONFIG_QUUX=42` + "\n")
	for i := 0; i < 1000; i++ {
		buf := new(bytes.Buffer)
		if _, err := cfg.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), wantsb.String(); got != want {
			t.Fatalf("%#v.WriteTo() => %q, want %q", cfg, got, want)
		}
	}
}

type applyDiffTest struct {
	Cfg     Config
	Diff    ConfigDiff
	Want    Config
	WantErr error
}

var applyDiffTests = []applyDiffTest{
	{
		Cfg: Config{
			"X": "x",
			"Y": "y",
			"Z": "z",
		},
		Diff: ConfigDiff{
			InOld: []ConfigValue{
				{Opt: "X", Val: "x"},
			},
			Changes: []ConfigChange{
				{Opt: "Y", OldVal: "y", NewVal: "yy"},
			},
			InNew: []ConfigValue{
				{Opt: "T", Val: "t"},
			},
		},
		Want: Config{
			"Y": "yy",
			"Z": "z",
			"T": "t",
		},
	},
	{
		Cfg: Config{
			"X": "x",
			"Y": "y",
			"Z": "z",
		},
		Diff: ConfigDiff{
			InOld: []ConfigValue{
				{Opt: "T", Val: "t"},
			},
		},
		WantErr: invalidOldValueError(ConfigValue{
			Opt: "T",
			Val: "t",
		}),
	},
	{
		Cfg: Config{
			"X": "x",
			"Y": "y",
			"Z": "z",
		},
		Diff: ConfigDiff{
			Changes: []ConfigChange{
				{Opt: "T", OldVal: "t", NewVal: "tt"},
			},
		},
		WantErr: invalidChangeError(ConfigChange{
			Opt:    "T",
			OldVal: "t",
			NewVal: "tt",
		}),
	},
	{
		Cfg: Config{
			"X": "x",
			"Y": "y",
			"Z": "z",
		},
		Diff: ConfigDiff{
			Changes: []ConfigChange{
				{Opt: "X", OldVal: "t", NewVal: "tt"},
			},
		},
		WantErr: mismatchedChangeError{
			cc: ConfigChange{
				Opt:    "X",
				OldVal: "t",
				NewVal: "tt",
			},
			oldval: "x",
		},
	},
	{
		Cfg: Config{
			"X": "x",
			"Y": "y",
			"Z": "z",
		},
		Diff: ConfigDiff{
			InNew: []ConfigValue{
				{Opt: "X", Val: "t"},
			},
		},
		WantErr: invalidNewValueError(ConfigValue{
			Opt: "X",
			Val: "t",
		}),
	},
}

func (tt applyDiffTest) Run(t *testing.T) {
	got, err := tt.Cfg.ApplyDiff(tt.Diff)
	if err != nil && tt.WantErr == nil {
		t.Fatalf("%#v.ApplyDiff(%#v): %v",
			tt.Cfg, tt.Diff, err)
	}
	if err == nil && tt.WantErr != nil {
		t.Fatalf("%#v.ApplyDiff(%#v): got %#v, want error %#v",
			tt.Cfg, tt.Diff, got, tt.WantErr)
	}
	if !got.Equal(tt.Want) {
		t.Fatalf("%#v.ApplyDiff(%#v): got %#v, want %#v",
			tt.Cfg, tt.Diff, got, tt.Want)
	}
	if err != tt.WantErr {
		t.Fatalf("%#v.ApplyDiff(%#v): got error %#v, want %#v",
			tt.Cfg, tt.Diff, err, tt.WantErr)
	}
	if tt.WantErr == nil {
		newdiff := DiffConfig(tt.Cfg, got)
		if !reflect.DeepEqual(newdiff, tt.Diff) {
			tt.failAsymmetric(t, got, newdiff)
		}
	}
}

func (tt applyDiffTest) failAsymmetric(t *testing.T, got Config, newdiff ConfigDiff) {
	t.Helper()
	t.Fatalf("%#v.ApplyDiff(%#v) => %#v. DiffConfig(%#v, %#v) => %#v, want %#v",
		tt.Cfg, tt.Diff, got, tt.Cfg, got, newdiff, tt.Diff)
}

func testApplyDiff(t *testing.T) {
	for _, tt := range applyDiffTests {
		tt.Run(t)
	}
}

var diffTests = []struct {
	Old, New Config
	Want     ConfigDiff
}{
	{
		Old: Config{
			"FOO": "42",
		},
		New: Config{
			"FOO": "42",
		},
		Want: ConfigDiff{},
	},
	{
		Old: Config{
			"FOO":  "4",
			"FOO2": "42",
			"BAR":  "n",
			"X":    "x",
			"Y":    "y",
			"QUUX": "23",
		},
		New: Config{
			"QUUX": "23",
			"BAZ":  "blah",
			"BAZ2": "blah2",
			"X":    "z",
			"Y":    "t",
			"BAR":  "y",
		},
		Want: ConfigDiff{
			InOld: []ConfigValue{
				{Opt: "FOO", Val: "4"},
				{Opt: "FOO2", Val: "42"},
			},
			Changes: []ConfigChange{
				{Opt: "BAR", OldVal: "n", NewVal: "y"},
				{Opt: "X", OldVal: "x", NewVal: "z"},
				{Opt: "Y", OldVal: "y", NewVal: "t"},
			},
			InNew: []ConfigValue{
				{Opt: "BAZ", Val: "blah"},
				{Opt: "BAZ2", Val: "blah2"},
			},
		},
	},
}

func testDiffConfigBasic(t *testing.T) {
	for _, tt := range diffTests {
		got := DiffConfig(tt.Old, tt.New)
		if !reflect.DeepEqual(got, tt.Want) {
			t.Fatalf("DiffConfig(%#v, %#v) = %#v, want %#v",
				tt.Old, tt.New, got, tt.Want)
		}
	}
}

func testDiffConfigSymmetry(t *testing.T) {
	for _, tt := range diffTests {
		gotOldNew := DiffConfig(tt.Old, tt.New)
		gotNewOld := DiffConfig(tt.New, tt.Old)
		if !reflect.DeepEqual(gotOldNew.InOld, gotNewOld.InNew) {
			t.Fatalf("DiffConfig(%#v, %#v) is not symmetric: %#v vs. %#v",
				tt.Old, tt.New, gotOldNew.InOld, gotNewOld.InNew)
		}
		if !reflect.DeepEqual(gotOldNew.InNew, gotNewOld.InOld) {
			t.Fatalf("DiffConfig(%#v, %#v) is not symmetric: %#v vs. %#v",
				tt.Old, tt.New, gotOldNew.InNew, gotNewOld.InOld)
		}
		for i, oldnewcc := range gotOldNew.Changes {
			newoldcc := gotNewOld.Changes[i]
			if !symmetricChanges(oldnewcc, newoldcc) {
				t.Fatalf("DiffConfig(%#v, %#v) is not symmetric: %#v vs. %#v",
					tt.Old, tt.New, oldnewcc, newoldcc)
			}
		}
	}
}

func symmetricChanges(oldnew, newold ConfigChange) bool {
	equalopts := oldnew.Opt == newold.Opt
	symmetricvals := oldnew.OldVal == newold.NewVal && oldnew.NewVal == newold.OldVal
	return equalopts && symmetricvals
}

var testConfigDiff = ConfigDiff{
	InOld: []ConfigValue{
		{Opt: "FOO", Val: "4"},
		{Opt: "FOO2", Val: "42"},
	},
	Changes: []ConfigChange{
		{Opt: "BAR", OldVal: "n", NewVal: "y"},
		{Opt: "Y", OldVal: "y", NewVal: "t"},
	},
	InNew: []ConfigValue{
		{Opt: "BAZ", Val: "blah"},
		{Opt: "BAZ2", Val: "blah2"},
	},
}

const testConfigDiffString = "-FOO 4\n-FOO2 42\n BAR n -> y\n Y y -> t\n+BAZ blah\n+BAZ2 blah2\n"

func testConfigDiffWriteTo(t *testing.T) {
	buf := new(bytes.Buffer)
	if _, err := testConfigDiff.WriteTo(buf); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), testConfigDiffString; got != want {
		t.Fatalf("%#v.WriteTo() => %q, want %q", testConfigDiff, got, want)
	}
}

func testConfigDiffWriteToPredictableOrder(t *testing.T) {
	for i := 0; i < 1000; i++ {
		buf := new(bytes.Buffer)
		if _, err := testConfigDiff.WriteTo(buf); err != nil {
			t.Fatal(err)
		}
		if got, want := buf.String(), testConfigDiffString; got != want {
			t.Fatalf("%#v.WriteTo() => %q, want %q", testConfigDiff, got, want)
		}
	}
}
