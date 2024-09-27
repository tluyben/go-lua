package lua

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
)

func relativePosition(pos, length int) int {
	if pos >= 0 {
		return pos
	} else if -pos > length {
		return 0
	}
	return length + pos + 1
}

func findHelper(l *State, isFind bool) int {
	s, p := CheckString(l, 1), CheckString(l, 2)
	init := relativePosition(OptInteger(l, 3, 1), len(s))
	if init < 1 {
		init = 1
	} else if init > len(s)+1 {
		l.PushNil()
		return 1
	}
	isPlain := l.TypeOf(4) == TypeNone || l.ToBoolean(4)
	if isFind && (isPlain || !strings.ContainsAny(p, "^$*+?.([%-")) {
		if start := strings.Index(s[init-1:], p); start >= 0 {
			l.PushInteger(start + init)
			l.PushInteger(start + init + len(p) - 1)
			return 2
		}
	} else {
		l.assert(false) // TODO implement pattern matching
	}
	l.PushNil()
	return 1
}
func gmatch(l *State) int {
	s := CheckString(l, 1)
	pattern := CheckString(l, 2)

	// Compile the Lua pattern into a Go regular expression
	goPattern, err := luaPatternToGoRegex(pattern)
	if err != nil {
		Errorf(l, "invalid pattern: %s", err.Error())
	}

	re, err := regexp.Compile(goPattern)
	if err != nil {
		Errorf(l, "invalid pattern: %s", err.Error())
	}

	// Create a closure to be returned as the iterator function
	l.PushGoClosure(func(l *State) int {
		// Find the next match
		match := re.FindStringSubmatch(s)
		if match == nil {
			l.PushNil() // No more matches
			return 1
		}

		// Update s to start after this match for the next iteration
		s = s[len(match[0]):]

		// Push capture groups or whole match
		if len(match) > 1 {
			for _, group := range match[1:] {
				l.PushString(group)
			}
			return len(match) - 1
		} else {
			l.PushString(match[0])
			return 1
		}
	}, 0)

	return 1
}


func luaPatternToGoRegex(pattern string) (string,error) {
	var goPattern strings.Builder
	
	escaped := false
	
	for _, ch := range pattern {
		if escaped {
			switch ch {
			case 'd':
				goPattern.WriteString("\\d")
			case 's':
				goPattern.WriteString("\\s")
			case 'w':
				goPattern.WriteString("\\w")
			case '%':
				goPattern.WriteRune('%')
			default:
				goPattern.WriteRune(ch)
			}
			escaped = false
		} else if ch == '%' {
			escaped = true
		} else if ch == '.' {
			goPattern.WriteString(".")
		} else {
			goPattern.WriteRune(ch)
		}
	}
	
	if escaped {
		goPattern.WriteRune('%')
	}
	
	return goPattern.String(), nil
}
func gsub(l *State) int {
    s := CheckString(l, 1)
    pattern := CheckString(l, 2)
    
    var replacement func([]string) string
    var maxReplacements int
    
    switch l.TypeOf(3) {
    case TypeString:
        repl := CheckString(l, 3)
        replacement = func(captures []string) string {
            return expandLuaReplacement(repl, captures)
        }
    case TypeFunction:
        replacement = func(captures []string) string {
            l.PushValue(3) // Push the function
            for _, capture := range captures {
                l.PushString(capture)
            }
            l.Call(len(captures), 1)
			result, _ := l.ToString(-1)
            l.Pop(1)
            return result
        }
    case TypeTable:
        replacement = func(captures []string) string {
            l.PushValue(3) // Push the table
            l.PushString(captures[0])
            l.Table(-2)
			result, _ := l.ToString(-1)
            l.Pop(2) // Pop result and table
            return result
        }
    default:
        Errorf(l, "string/function/table expected, got %s", l.TypeOf(3).String())
    }
    
    if l.IsNumber(4) {
        maxReplacements = int(CheckNumber(l, 4))
    } else {
        maxReplacements = -1 // No limit
    }
    
    // Compile the Lua pattern into a Go regular expression
    goPattern, err := luaPatternToGoRegex(pattern)
    if err != nil {
        Errorf(l, "invalid pattern: %s", err.Error())
    }
    re, err := regexp.Compile(goPattern)
    if err != nil {
        Errorf(l, "invalid pattern: %s", err.Error())
    }
    
	replacementCount := 0
    result := re.ReplaceAllStringFunc(s, func(match string) string {
        if maxReplacements >= 0 && replacementCount >= maxReplacements {
            return match
        }
        replacementCount++
        
        captures := re.FindStringSubmatch(match)
        return replacement(captures)
    })
    
    l.PushString(result)
	l.PushInteger(int(replacementCount))
    return 2
}

func expandLuaReplacement(repl string, captures []string) string {
    var result strings.Builder
    escaped := false
    
    for _, ch := range repl {
        if escaped {
            if ch >= '1' && ch <= '9' {
                captureIndex := int(ch - '0')
                if captureIndex < len(captures) {
                    result.WriteString(captures[captureIndex])
                }
            } else if ch == '0' {
                result.WriteString(captures[0])
            } else {
                result.WriteRune(ch)
            }
            escaped = false
        } else if ch == '%' {
            escaped = true
        } else {
            result.WriteRune(ch)
        }
    }
    
    if escaped {
        result.WriteRune('%')
    }
    
    return result.String()
}
func strMatch(l *State) int {
    s := CheckString(l, 1)
    pattern := CheckString(l, 2)
    init := OptInteger(l, 3, 1) - 1 // Lua is 1-indexed, Go is 0-indexed

    if init < 0 {
        init = 0
    } else if init > len(s) {
        l.PushNil()
        return 1
    }

    // Compile the Lua pattern into a Go regular expression
    goPattern, err := luaPatternToGoRegex(pattern)
    if err != nil {
        Errorf(l, "invalid pattern: %s", err.Error())
    }
    re, err := regexp.Compile(goPattern)
    if err != nil {
        Errorf(l, "invalid pattern: %s", err.Error())
    }

    // Find the first match
    match := re.FindStringSubmatchIndex(s[init:])
    if match == nil {
        l.PushNil()
        return 1
    }

    // Adjust match indices for the 'init' offset
    for i := range match {
        match[i] += init
    }

    // Push captures or whole match
    if len(match) > 2 {
        for i := 2; i < len(match); i += 2 {
            if match[i] != -1 {
                l.PushString(s[match[i]:match[i+1]])
            } else {
                l.PushString("") // Push empty string for unmatched optional captures
            }
        }
        return len(match)/2 - 1
    } else {
        l.PushString(s[match[0]:match[1]])
        return 1
    }
}
func scanFormat(l *State, fs string) string {
	i := 0
	skipDigit := func() {
		if unicode.IsDigit(rune(fs[i])) {
			i++
		}
	}
	flags := "-+ #0"
	for i < len(fs) && strings.ContainsRune(flags, rune(fs[i])) {
		i++
	}
	if i >= len(flags) {
		Errorf(l, "invalid format (repeated flags)")
	}
	skipDigit()
	skipDigit()
	if fs[i] == '.' {
		i++
		skipDigit()
		skipDigit()
	}
	if unicode.IsDigit(rune(fs[i])) {
		Errorf(l, "invalid format (width or precision too long)")
	}
	i++
	return "%" + fs[:i]
}

func formatHelper(l *State, fs string, argCount int) string {
	var b bytes.Buffer
	for i, arg := 0, 1; i < len(fs); i++ {
		if fs[i] != '%' {
			b.WriteByte(fs[i])
		} else if i++; fs[i] == '%' {
			b.WriteByte(fs[i])
		} else {
			if arg++; arg > argCount {
				ArgumentError(l, arg, "no value")
			}
			f := scanFormat(l, fs[i:])
			switch i += len(f) - 2; fs[i] {
			case 'c':
				// Ensure each character is represented by a single byte, while preserving format modifiers.
				c := CheckInteger(l, arg)
				fmt.Fprintf(&b, f, 'x')
				buf := b.Bytes()
				buf[len(buf)-1] = byte(c)
			case 'i': // The fmt package doesn't support %i.
				f = f[:len(f)-1] + "d"
				fallthrough
			case 'd':
				n := CheckNumber(l, arg)
				ArgumentCheck(l, math.Floor(n) == n && -math.Pow(2, 63) <= n && n < math.Pow(2, 63), arg, "number has no integer representation")
				ni := int(n)
				fmt.Fprintf(&b, f, ni)
			case 'u': // The fmt package doesn't support %u.
				f = f[:len(f)-1] + "d"
				n := CheckNumber(l, arg)
				ArgumentCheck(l, math.Floor(n) == n && 0.0 <= n && n < math.Pow(2, 64), arg, "not a non-negative number in proper range")
				ni := uint(n)
				fmt.Fprintf(&b, f, ni)
			case 'o', 'x', 'X':
				n := CheckNumber(l, arg)
				ArgumentCheck(l, 0.0 <= n && n < math.Pow(2, 64), arg, "not a non-negative number in proper range")
				ni := uint(n)
				fmt.Fprintf(&b, f, ni)
			case 'e', 'E', 'f', 'g', 'G':
				fmt.Fprintf(&b, f, CheckNumber(l, arg))
			case 'q':
				s := CheckString(l, arg)
				b.WriteByte('"')
				for i := 0; i < len(s); i++ {
					switch s[i] {
					case '"', '\\', '\n':
						b.WriteByte('\\')
						b.WriteByte(s[i])
					default:
						if 0x20 <= s[i] && s[i] != 0x7f { // ASCII control characters don't correspond to a Unicode range.
							b.WriteByte(s[i])
						} else if i+1 < len(s) && unicode.IsDigit(rune(s[i+1])) {
							fmt.Fprintf(&b, "\\%03d", s[i])
						} else {
							fmt.Fprintf(&b, "\\%d", s[i])
						}
					}
				}
				b.WriteByte('"')
			case 's':
				if s, _ := ToStringMeta(l, arg); !strings.ContainsRune(f, '.') && len(s) >= 100 {
					b.WriteString(s)
				} else {
					fmt.Fprintf(&b, f, s)
				}
			default:
				Errorf(l, fmt.Sprintf("invalid option '%%%c' to 'format'", fs[i]))
			}
		}
	}
	return b.String()
}

var stringLibrary = []RegistryFunction{
	{"byte", func(l *State) int {
		s := CheckString(l, 1)
		start := relativePosition(OptInteger(l, 2, 1), len(s))
		end := relativePosition(OptInteger(l, 3, start), len(s))
		if start < 1 {
			start = 1
		}
		if end > len(s) {
			end = len(s)
		}
		if start > end {
			return 0
		}
		n := end - start + 1
		if start+n <= end {
			Errorf(l, "string slice too long")
		}
		CheckStackWithMessage(l, n, "string slice too long")
		for _, c := range []byte(s[start-1 : end]) {
			l.PushInteger(int(c))
		}
		return n
	}},
	{"char", func(l *State) int {
		var b bytes.Buffer
		for i, n := 1, l.Top(); i <= n; i++ {
			c := CheckInteger(l, i)
			ArgumentCheck(l, int(byte(c)) == c, i, "value out of range")
			b.WriteByte(byte(c))
		}
		l.PushString(b.String())
		return 1
	}},
	// {"dump", ...},
	{"find", func(l *State) int { return findHelper(l, true) }},
	{"format", func(l *State) int {
		l.PushString(formatHelper(l, CheckString(l, 1), l.Top()))
		return 1
	}},
	{"gmatch", gmatch},
	{"gsub", gsub},
	{"len", func(l *State) int { l.PushInteger(len(CheckString(l, 1))); return 1 }},
	{"lower", func(l *State) int { l.PushString(strings.ToLower(CheckString(l, 1))); return 1 }},
	{"match", strMatch},
	{"rep", func(l *State) int {
		s, n, sep := CheckString(l, 1), CheckInteger(l, 2), OptString(l, 3, "")
		if n <= 0 {
			l.PushString("")
		} else if len(s)+len(sep) < len(s) || len(s)+len(sep) >= maxInt/n {
			Errorf(l, "resulting string too large")
		} else if sep == "" {
			l.PushString(strings.Repeat(s, n))
		} else {
			var b bytes.Buffer
			b.Grow(n*len(s) + (n-1)*len(sep))
			b.WriteString(s)
			for ; n > 1; n-- {
				b.WriteString(sep)
				b.WriteString(s)
			}
			l.PushString(b.String())
		}
		return 1
	}},
	{"reverse", func(l *State) int {
		r := []rune(CheckString(l, 1))
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		l.PushString(string(r))
		return 1
	}},
	{"sub", func(l *State) int {
		s := CheckString(l, 1)
		start, end := relativePosition(CheckInteger(l, 2), len(s)), relativePosition(OptInteger(l, 3, -1), len(s))
		if start < 1 {
			start = 1
		}
		if end > len(s) {
			end = len(s)
		}
		if start <= end {
			l.PushString(s[start-1 : end])
		} else {
			l.PushString("")
		}
		return 1
	}},
	{"upper", func(l *State) int { l.PushString(strings.ToUpper(CheckString(l, 1))); return 1 }},
}

// StringOpen opens the string library. Usually passed to Require.
func StringOpen(l *State) int {
	NewLibrary(l, stringLibrary)
	l.CreateTable(0, 1)
	l.PushString("")
	l.PushValue(-2)
	l.SetMetaTable(-2)
	l.Pop(1)
	l.PushValue(-2)
	l.SetField(-2, "__index")
	l.Pop(1)
	return 1
}
