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
	"io"
	"sort"
	"strings"
)

// Config represents a parsed Linux kernel configuration file.
//
// Given a value cfg of type Config:
//
// * for CONFIG_X=something, cfg["X"] == "bar"
//
// * for CONFIG_Y="", cfg["Y"] == "".
//
// * for # CONFIG_Z is not set, cfg["Z"] == "n".
type Config map[string]string

// ParseConfig parses a Config from r. It reads from r until EOF. ParseConfig
// assumes that its input is a well-formed kernel configuration file: its
// behavior is undefined otherwise.
func ParseConfig(r io.Reader) (Config, error) {
	cfg := make(Config)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		opt, val := parseConfigLine(sc.Text())
		if opt != "" {
			cfg[opt] = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseConfigLine parses a line from a kernel config file.
// It returns the option and the corresponding value, if any.
//
// For the line "CONFIG_HAVE_KERNEL_GZIP=y" it returns the pair
// "HAVE_KERNEL_GZIP", "y".
//
// For the line "# CONFIG_COMPILE_TEST is not set", it returns the pair
// "COMPILE_TEST", "n".
//
// For any other types of lines, such as "# General setup", it returns
// empty strings.
func parseConfigLine(line string) (opt, val string) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "CONFIG_") {
		line = strings.TrimPrefix(line, "CONFIG_")
		tokens := strings.Split(line, "=")
		return tokens[0], tokens[1]
	}
	if strings.HasSuffix(line, " is not set") {
		line = strings.TrimSuffix(line, " is not set")
		line = strings.TrimPrefix(line, "# CONFIG_")
		return line, "n"
	}
	return "", ""
}

// WriteTo writes cfg to w in deterministic order. The output is a valid
// kernel configuration file, but it is almost certainly different from
// the original file the Config was parsed from.
func (cfg Config) WriteTo(w io.Writer) (int64, error) {
	opts := make([]string, 0, len(cfg))
	for opt := range cfg {
		opts = append(opts, opt)
	}
	sort.Strings(opts)
	cfgw := &configWriter{W: w}
	for _, opt := range opts {
		cfgw.WriteLine(opt, cfg[opt])
	}
	return cfgw.N, cfgw.Err
}

// Equal returns a boolean indicating whether cfg and the specified config
// are equal, i.e. the two configurations contain the exact same set of keys,
// and all the corresponding values are equal.
//
// Comparing configuration files from different kernel versions may or may
// not be meaningful.
func (cfg Config) Equal(other Config) bool {
	return cfg.containedIn(other) && other.containedIn(cfg)
}

// containedIn returns a boolean indicating whether all the options in cfg are
// contained in the specified Config, and all the corresponding values match.
func (cfg Config) containedIn(other Config) bool {
	for opt, val := range cfg {
		otherval, ok := other[opt]
		if !ok || val != otherval {
			return false
		}
	}
	return true
}

// ApplyDiff applies the specified diff to cfg and returns a new config, such
// that, schematically, if x.ApplyDiff(d) == y, then DiffConfig(x, y) == d.
func (cfg Config) ApplyDiff(diff ConfigDiff) (Config, error) {
	new := make(Config, len(cfg))
	for opt, val := range cfg {
		new[opt] = val
	}
	for _, cv := range diff.InOld {
		if _, ok := cfg[cv.Opt]; !ok {
			return nil, invalidOldValueError(cv)
		}
		delete(new, cv.Opt)
	}
	for _, cc := range diff.Changes {
		oldval, ok := cfg[cc.Opt]
		if !ok {
			return nil, invalidChangeError(cc)
		}
		if oldval != cc.OldVal {
			return nil, mismatchedChangeError{cc: cc, oldval: oldval}
		}
		new[cc.Opt] = cc.NewVal
	}
	for _, cv := range diff.InNew {
		if _, ok := cfg[cv.Opt]; ok {
			return nil, invalidNewValueError(cv)
		}
		new[cv.Opt] = cv.Val
	}
	return new, nil
}

// DiffConfig returns the differences between the old and new config.
func DiffConfig(old, new Config) ConfigDiff {
	diff := ConfigDiff{}
	for opt, oldval := range old {
		newval, ok := new[opt]
		if !ok {
			diff.InOld = append(diff.InOld, ConfigValue{
				Opt: opt,
				Val: oldval,
			})
		} else if oldval != newval {
			diff.Changes = append(diff.Changes, ConfigChange{
				Opt:    opt,
				OldVal: oldval,
				NewVal: newval,
			})
		}
	}
	for opt, newval := range new {
		if _, ok := old[opt]; !ok {
			diff.InNew = append(diff.InNew, ConfigValue{
				Opt: opt,
				Val: newval,
			})
		}
	}
	diff.sort()
	return diff
}

// ConfigDiff contains differences between two kernel configurations. The
// slices are sorted by the option name.
type ConfigDiff struct {
	InOld   []ConfigValue
	Changes []ConfigChange
	InNew   []ConfigValue
}

func (diff ConfigDiff) sort() {
	sort.Slice(diff.InOld, func(i, j int) bool {
		return diff.InOld[i].Opt < diff.InOld[j].Opt
	})
	sort.Slice(diff.Changes, func(i, j int) bool {
		return diff.Changes[i].Opt < diff.Changes[j].Opt
	})
	sort.Slice(diff.InNew, func(i, j int) bool {
		return diff.InNew[i].Opt < diff.InNew[j].Opt
	})
}

// WriteTo writes the diff to w in a similar format to the scripts/diffconfig
// tool distributed with the Linux kernel.
//
// The order is predictable: InOld, Changes, InNew.
func (diff ConfigDiff) WriteTo(w io.Writer) (int64, error) {
	cfgdw := &configDiffWriter{W: w}
	for _, cv := range diff.InOld {
		cfgdw.WriteOld(cv)
	}
	for _, cc := range diff.Changes {
		cfgdw.WriteChange(cc)
	}
	for _, cv := range diff.InNew {
		cfgdw.WriteNew(cv)
	}
	return cfgdw.N, cfgdw.Err
}

// ConfigValue contains an option, value pair.
type ConfigValue struct {
	Opt, Val string
}

// String formats vs as, for example: "PKCS8_PRIVATE_KEY_PARSER n".
func (cv ConfigValue) String() string {
	return fmt.Sprintf("%s %s", cv.Opt, cv.Val)
}

// ConfigChange specifies a change in a configuration value.
type ConfigChange struct {
	Opt, OldVal, NewVal string
}

// String formats cc as, for example: "INET6_ESP_OFFLOAD n -> m".
func (cc ConfigChange) String() string {
	return fmt.Sprintf("%s %s -> %s", cc.Opt, cc.OldVal, cc.NewVal)
}

type configWriter struct {
	W   io.Writer
	N   int64
	Err error // sticky
}

func (cfgw *configWriter) WriteLine(option, value string) {
	if cfgw.Err != nil {
		return
	}
	var n int
	switch value {
	case "n":
		n, cfgw.Err = fmt.Fprintf(cfgw.W, "# CONFIG_%s is not set\n", option)
	default:
		n, cfgw.Err = fmt.Fprintf(cfgw.W, "CONFIG_%s=%s\n", option, value)
	}
	cfgw.N += int64(n)
}

type configDiffWriter struct {
	W   io.Writer
	N   int64
	Err error // sticky
}

func (cfgdw *configDiffWriter) WriteOld(cv ConfigValue) {
	if cfgdw.Err != nil {
		return
	}
	var n int
	n, cfgdw.Err = fmt.Fprintf(cfgdw.W, "-%s\n", cv)
	cfgdw.N += int64(n)
}

func (cfgdw *configDiffWriter) WriteChange(cc ConfigChange) {
	if cfgdw.Err != nil {
		return
	}
	var n int
	n, cfgdw.Err = fmt.Fprintf(cfgdw.W, " %s\n", cc)
	cfgdw.N += int64(n)
}

func (cfgdw *configDiffWriter) WriteNew(cv ConfigValue) {
	if cfgdw.Err != nil {
		return
	}
	var n int
	n, cfgdw.Err = fmt.Fprintf(cfgdw.W, "+%s\n", cv)
	cfgdw.N += int64(n)
}

type invalidOldValueError ConfigValue

func (cv invalidOldValueError) Error() string {
	return fmt.Sprintf("cannot apply diff: %q in diff.Old, but no %qin cfg",
		ConfigValue(cv), cv.Opt)
}

type invalidChangeError ConfigChange

func (cc invalidChangeError) Error() string {
	return fmt.Sprintf("cannot apply diff: %q in diff.Changes, but no %q in cfg",
		ConfigChange(cc), cc.Opt)
}

type mismatchedChangeError struct {
	cc     ConfigChange
	oldval string
}

func (e mismatchedChangeError) Error() string {
	return fmt.Sprintf("cannot apply diff: %q in diff.Changes, but old %q value is %q",
		e.cc, e.cc.Opt, e.oldval)
}

type invalidNewValueError ConfigValue

func (cv invalidNewValueError) Error() string {
	return fmt.Sprintf("cannot apply diff: %q in diff.InNew, but %q in cfg",
		ConfigValue(cv), cv.Opt)
}
