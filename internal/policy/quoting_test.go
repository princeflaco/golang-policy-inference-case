package policy

import (
	"reflect"
	"testing"
)

func TestSplitOutsideQuotes(t *testing.T) {
	tests := []struct {
		in   string
		sep  byte
		n    int
		want []string
		ok   bool
	}{
		{`a=1,b="x,y",c=2`, ',', -1, []string{`a=1`, `b="x,y"`, `c=2`}, true},
		{`b="a\",b",c=1`, ',', -1, []string{`b="a\",b"`, `c=1`}, true},
		{`k="a=b"=c`, '=', 2, []string{`k`, `"a=b"=c`}, true},
		{`only`, '=', 2, []string{`only`}, true},
		{`s="open`, ',', -1, nil, false},
		{`s="trailing\`, ',', -1, nil, false},
	}
	for _, test := range tests {
		got, err := splitOutsideQuotes(test.in, test.sep, test.n)
		if (err == nil) != test.ok || !reflect.DeepEqual(got, test.want) {
			t.Errorf("splitOutsideQuotes(%q) = %q, %v", test.in, got, err)
		}
	}
}

func TestUnquotePolicyString(t *testing.T) {
	tests := map[string]struct {
		want string
		ok   bool
	}{
		`""`:            {"", true},
		`"a\"b\\c"`:     {`a"b\c`, true},
		`"tab\tline\n"`: {"tab\tline\n", true},
		`"a"b"`:         {"", false},
		"\"a\nb\"":      {"", false},
		`"bad\r"`:       {"", false},
		`"open\"`:       {"", false},
		`unquoted`:      {"", false},
	}
	for in, test := range tests {
		got, err := unquotePolicyString(in)
		if (err == nil) != test.ok || got != test.want {
			t.Errorf("unquotePolicyString(%q) = %q, %v", in, got, err)
		}
	}
}
