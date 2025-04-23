package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// parseValue attempts to convert a string into int64, float64, bool, or returns string
func parseValue(val string) interface{} {
	if i, err := strconv.ParseInt(val, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	if val == "true" {
		return true
	}
	if val == "false" {
		return false
	}
	if val == "null" {
		return nil
	}
	return val
}

// compare handles =, !=, <, >, <=, >= and regex ~=, !~
func compare(actual interface{}, op string, expected interface{}) bool {
	actualStr := fmt.Sprintf("%v", actual)
	expectedStr := fmt.Sprintf("%v", expected)

	switch op {
	case "=":
		return actualStr == expectedStr
	case "!=":
		return actualStr != expectedStr
	case "<", "<=", ">", ">=":
		actF, aok := toFloat(actual)
		expF, eok := toFloat(expected)
		if aok && eok {
			switch op {
			case "<":
				return actF < expF
			case "<=" :
				return actF <= expF
			case ">":
				return actF > expF
			case ">=":
				return actF >= expF
			}
		}
	case "~":
		matched, _ := regexp.MatchString(expectedStr, actualStr)
		return matched
	case "!~":
		matched, _ := regexp.MatchString(expectedStr, actualStr)
		return !matched
	}
	return false
}

func toFloat(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// matchesAnyWhereBlock now supports extended operators in --where
func matchesAnyWhereBlock(obj *unstructured.Unstructured, whereClauses []string) bool {
	re := regexp.MustCompile(`(?P<key>.+?)(?P<op>>=|<=|!=|=|<|>|~|!~)(?P<value>.+)`)
	for _, clause := range whereClauses {
		ands := strings.Split(clause, ",")
		allMatch := true
		for _, cond := range ands {
			match := re.FindStringSubmatch(cond)
			if len(match) != 4 {
				fmt.Printf("⚠️ Skipping invalid condition: %s\n", cond)
				allMatch = false
				break
			}
			key, op, val := match[1], match[2], parseValue(match[3])
			keyParts := strings.Split(key, ".")
			actual, found, _ := unstructured.NestedFieldCopy(obj.Object, keyParts...)
			if !found || !compare(actual, op, val) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

