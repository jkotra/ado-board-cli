package tui

import (
	"regexp"
	"strings"
	"sync"
)

var (
	identityLock     sync.RWMutex
	globalIdentities = make(map[string]string)

	adoMentionGuidRegex      = regexp.MustCompile(`@<([0-9a-fA-F\-]{36})>`)
	adoMentionNamedRegex     = regexp.MustCompile(`@<([^>]+)>`)
	adoMentionWithEmailRegex = regexp.MustCompile(`(?i)@([^\n<@]+?)\s*<([a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,})>`)
	adoHtmlMentionRegex      = regexp.MustCompile(`(?i)<a[^>]*data-vss-mention[^>]*>(?:<[^>]+>)*([^<]+)(?:<[^>]+>)*</a>`)
	adoHtmlClassMentionRegex = regexp.MustCompile(`(?i)<(?:a|span)[^>]*class="[^"]*mention[^"]*"[^>]*>(?:<[^>]+>)*([^<]+)(?:<[^>]+>)*</(?:a|span)>`)
)

// SetGlobalIdentities populates the cached identities mapping
func SetGlobalIdentities(identities map[string]string) {
	identityLock.Lock()
	defer identityLock.Unlock()
	for k, v := range identities {
		globalIdentities[strings.ToLower(strings.TrimSpace(k))] = v
	}
}

// RegisterIdentity registers a single identity into the cache
func RegisterIdentity(idOrEmail, displayName string) {
	if idOrEmail == "" || displayName == "" {
		return
	}
	identityLock.Lock()
	defer identityLock.Unlock()
	globalIdentities[strings.ToLower(strings.TrimSpace(idOrEmail))] = displayName
}

// LookupIdentity looks up a display name for a given GUID or email
func LookupIdentity(key string) (string, bool) {
	identityLock.RLock()
	defer identityLock.RUnlock()
	name, ok := globalIdentities[strings.ToLower(strings.TrimSpace(key))]
	return name, ok
}

// ResolveMentions transforms ADO mention tokens into readable @DisplayName strings
func ResolveMentions(input string) string {
	if strings.TrimSpace(input) == "" {
		return input
	}

	// 1. Resolve HTML data-vss-mention links: <a data-vss-mention="...">@Name</a> -> **@Name**
	s := adoHtmlMentionRegex.ReplaceAllStringFunc(input, func(m string) string {
		match := adoHtmlMentionRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if !strings.HasPrefix(name, "@") {
				name = "@" + name
			}
			return "**" + name + "**"
		}
		return m
	})

	// 2. Resolve HTML class="mention" spans/links
	s = adoHtmlClassMentionRegex.ReplaceAllStringFunc(s, func(m string) string {
		match := adoHtmlClassMentionRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if !strings.HasPrefix(name, "@") {
				name = "@" + name
			}
			return "**" + name + "**"
		}
		return m
	})

	// 3. Resolve GUID mentions: @<0F433BE8-555E-621F-8ADB-94531C9BB685>
	s = adoMentionGuidRegex.ReplaceAllStringFunc(s, func(m string) string {
		match := adoMentionGuidRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			guid := match[1]
			if name, found := LookupIdentity(guid); found {
				return "**@" + name + "**"
			}
			// If not found in cache, display as @User
			return "**@User**"
		}
		return m
	})

	// 4. Resolve Named/Email in brackets: @<alice.anderson@example.com> or @<Alice Anderson>
	s = adoMentionNamedRegex.ReplaceAllStringFunc(s, func(m string) string {
		match := adoMentionNamedRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			val := strings.TrimSpace(match[1])
			if name, found := LookupIdentity(val); found {
				return "**@" + name + "**"
			}
			return "**@" + val + "**"
		}
		return m
	})

	// 5. Resolve typed @DisplayName <email>
	s = adoMentionWithEmailRegex.ReplaceAllStringFunc(s, func(m string) string {
		match := adoMentionWithEmailRegex.FindStringSubmatch(m)
		if len(match) > 1 {
			name := strings.TrimSpace(match[1])
			if !strings.HasPrefix(name, "@") {
				name = "@" + name
			}
			return "**" + name + "**"
		}
		return m
	})

	return s
}
