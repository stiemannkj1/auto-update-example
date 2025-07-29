// Common utilities shared between the Pokemon CLI and server
package common

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"
)

func SemVerMustParse(version string, t *testing.T) SemVer {
	semVer, err := ParseSemVer(version)

	if err != nil {
		t.Errorf("failed to parse semver %v", err)
		return SemVer{}
	}

	return semVer
}

func TestDoesNotParseInvalidSemVer(t *testing.T) {

	for _, testCase := range []string{
		"",
		"foo",
		"9..9..19",
		"1234.1234.",
		".1234.1234.",
		".1234.1234",
		"1234.1234.1234.",
		".1234.1234.1234.",
		".1234.1234.1234",
		"a.b.1234",
		"a.1234.1234",
		"a1234.1234.1234",
		"1234.1234.1234a",
		"1234567890",
		"1234.12345",
	} {

		_, err := ParseSemVer(testCase)

		if err == nil {
			t.Errorf("Expected error parsing %s", testCase)
		}
	}
}

func TestParseSemVer(t *testing.T) {

	type TestCase struct {
		expected      SemVer
		stringVersion string
	}

	for _, testCase := range []TestCase{
		{
			expected: SemVer{
				Major:  1,
				Minor:  0,
				Patch:  0,
				String: "1.0.0",
			},
			stringVersion: "1.0.0",
		},
		{
			expected: SemVer{
				Major:  1,
				Minor:  2,
				Patch:  3,
				String: "1.2.3",
			},
			stringVersion: "1.2.3",
		},
		{
			expected: SemVer{
				Major:  105,
				Minor:  256,
				Patch:  391,
				String: "105.256.391",
			},
			stringVersion: "105.256.391",
		},
		{
			expected: SemVer{
				Major:  9999999,
				Minor:  0,
				Patch:  11111,
				String: "9999999.0.11111",
			},
			stringVersion: "9999999.0.11111",
		},
	} {

		semVer, err := ParseSemVer(testCase.stringVersion)

		if err != nil {
			t.Errorf("%v", err)
		}

		if testCase.expected != semVer {
			t.Errorf("semVer did not parse correctly. Expected %+v but found %+v", testCase.expected, semVer)
		}
	}
}

func TestSemVerString(t *testing.T) {

	versions := SemanticVersions{
		All: []SemVer{
			SemVerMustParse("1.0.0", t),
			SemVerMustParse("2.0.0", t),
			SemVerMustParse("3.0.0", t),
		},
	}

	expected := "[1.0.0,2.0.0,3.0.0]"
	actual := versions.String()

	if expected != actual {
		t.Errorf("Expected %s but found %s", expected, actual)
	}
}

func TestSemVerJsonSerialization(t *testing.T) {

	expected := []byte("[\"1.0.0\",\"2.0.0\",\"3.0.0\"]")

	result, err := json.Marshal([]SemVer{
		SemVerMustParse("1.0.0", t),
		SemVerMustParse("2.0.0", t),
		SemVerMustParse("3.0.0", t),
	})

	if err != nil {
		t.Errorf("%v", err)
	}

	if !slices.Equal(expected, result) {
		t.Errorf("not equal:\n<[%s]>\n<[%s]>\n", result, expected)
	}

	expected = []byte("{\"versions\":[\"1.0.0\",\"2.0.0\",\"3.0.0\"]}")

	result, err = json.Marshal(SemanticVersions{
		All: []SemVer{
			SemVerMustParse("1.0.0", t),
			SemVerMustParse("2.0.0", t),
			SemVerMustParse("3.0.0", t),
		},
	})

	if err != nil {
		t.Errorf("%v", err)
	}

	if !slices.Equal(expected, result) {
		t.Errorf("not equal:\n<[%s]>\n<[%s]>\n", result, expected)
	}

	result, err = json.Marshal(Versions{
		All: []string{
			"1.0.0",
			"2.0.0",
			"3.0.0",
		},
	})

	if err != nil {
		t.Errorf("%v", err)
	}

	if !slices.Equal(expected, result) {
		t.Errorf("not equal:\n<[%s]>\n<[%s]>\n", result, expected)
	}
}

func TestSemVersSort(t *testing.T) {

	type TestCase struct {
		expected SemVers
		unsorted SemVers
	}

	for _, testCase := range []TestCase{
		{
			expected: []SemVer{
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("3.0.0", t),
			},
			unsorted: []SemVer{
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("3.0.0", t),
			},
		},
		{
			expected: []SemVer{
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("3.0.0", t),
			},
			unsorted: []SemVer{
				SemVerMustParse("3.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("1.0.0", t),
			},
		},
		{
			expected: []SemVer{
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("3.0.0", t),
			},
			unsorted: []SemVer{
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("3.0.0", t),
				SemVerMustParse("1.0.0", t),
			},
		},
		{
			expected: []SemVer{
				SemVerMustParse("0.0.0", t),
				SemVerMustParse("0.0.1", t),
				SemVerMustParse("0.0.2", t),
				SemVerMustParse("0.0.5", t),
				SemVerMustParse("0.0.9", t),
				SemVerMustParse("0.0.10", t),
				SemVerMustParse("0.0.11", t),
				SemVerMustParse("0.0.300", t),
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("2.90.0", t),
				SemVerMustParse("3.0.0", t),
				SemVerMustParse("176.243.789", t),
				SemVerMustParse("177.243.789", t),
				SemVerMustParse("200.0.0", t),
				SemVerMustParse("300.0.0", t),
			},
			unsorted: []SemVer{
				SemVerMustParse("0.0.0", t),
				SemVerMustParse("0.0.1", t),
				SemVerMustParse("0.0.10", t),
				SemVerMustParse("0.0.11", t),
				SemVerMustParse("0.0.2", t),
				SemVerMustParse("0.0.300", t),
				SemVerMustParse("0.0.5", t),
				SemVerMustParse("0.0.9", t),
				SemVerMustParse("1.0.0", t),
				SemVerMustParse("176.243.789", t),
				SemVerMustParse("177.243.789", t),
				SemVerMustParse("2.0.0", t),
				SemVerMustParse("2.90.0", t),
				SemVerMustParse("200.0.0", t),
				SemVerMustParse("3.0.0", t),
				SemVerMustParse("300.0.0", t),
			},
		},
	} {
		sort.Sort(testCase.unsorted)

		if !slices.Equal(testCase.expected, testCase.unsorted) {
			t.Errorf("semVer did not sort correctly. Expected %+v but found %+v", testCase.expected, testCase.unsorted)
		}
	}
}
