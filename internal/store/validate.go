// Shared input validation: the field limits are domain rules enforced at
// every service boundary — the HTML forms and the JSON API (/api/v1) call
// the same functions so their validation cannot drift apart
// (docs/sketch/ja/cli-and-web-api.md, 検証 section).
package store

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

const (
	// MaxTitleRunes is the title length limit for issues and discussions.
	MaxTitleRunes = 200
	// MaxBodyRunes is the length limit for bodies and comments.
	MaxBodyRunes = 10000
)

// FieldError is a single field's validation failure. Code is a stable
// machine key the web layer translates to the page locale; Limit carries
// the bound for "too_long" errors; Message is the default English text
// the JSON API returns as is.
type FieldError struct {
	Code    string
	Limit   int
	Message string
}

// Field error codes — the presentation layer maps them to localized
// messages (see internal/web's fieldErrText).
const (
	ErrCodeRequired    = "required"
	ErrCodeTooLong     = "too_long"
	ErrCodeSlugPattern = "pattern_slug"
	ErrCodeHexColor    = "hex_color"
)

// FieldErrors maps an input field name ("title", "body", "category") to a
// validation failure. nil means the input is valid.
type FieldErrors map[string]FieldError

// Any reports whether any field failed validation.
func (e FieldErrors) Any() bool { return len(e) > 0 }

func required(msg string) FieldError { return FieldError{Code: ErrCodeRequired, Message: msg} }

func tooLong(msg string, limit int) FieldError {
	return FieldError{Code: ErrCodeTooLong, Limit: limit, Message: msg}
}

// ValidateIssueInput validates an IssueInput value.
func ValidateIssueInput(in IssueInput) FieldErrors {
	var errs FieldErrors
	switch {
	case in.Title == "":
		errs = FieldErrors{"title": required("Title is required.")}
	case utf8.RuneCountInString(in.Title) > MaxTitleRunes:
		errs = FieldErrors{"title": tooLong(fmt.Sprintf("Title must be %d characters or fewer.", MaxTitleRunes), MaxTitleRunes)}
	}
	if utf8.RuneCountInString(in.Body) > MaxBodyRunes {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["body"] = tooLong(fmt.Sprintf("Body must be %d characters or fewer.", MaxBodyRunes), MaxBodyRunes)
	}
	return errs
}

// ValidateDiscussionInput validates a DiscussionInput value. A discussion
// must belong to exactly one category; the caller checks the category
// exists via the store.
func ValidateDiscussionInput(in DiscussionInput) FieldErrors {
	errs := ValidateIssueInput(IssueInput{Title: in.Title, Body: in.Body})
	if in.CategoryID == 0 {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["category"] = required("Category is required.")
	}
	return errs
}

// MaxLabelNameRunes is the label name length limit.
const MaxLabelNameRunes = 64

// labelColorRE is the color rule for labels — they render through the
// --label-color CSS custom property, so only a 6-digit hex color is
// accepted.
var labelColorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidateLabelInput validates a LabelInput value.
func ValidateLabelInput(in LabelInput) FieldErrors {
	var errs FieldErrors
	switch {
	case in.Name == "":
		errs = FieldErrors{"name": required("Name is required.")}
	case utf8.RuneCountInString(in.Name) > MaxLabelNameRunes:
		errs = FieldErrors{"name": tooLong(fmt.Sprintf("Name must be %d characters or fewer.", MaxLabelNameRunes), MaxLabelNameRunes)}
	}
	if !labelColorRE.MatchString(in.Color) {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["color"] = FieldError{Code: ErrCodeHexColor, Message: "Color must be a hex color like #d73a4a."}
	}
	return errs
}

// workspaceSlugRE is the identity rule for workspaces — the slug is the
// URL path segment and API key, so it stays lowercase ASCII.
var workspaceSlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// WorkspaceSlugPattern exposes the slug regex text for error messages.
func WorkspaceSlugPattern() string { return workspaceSlugRE.String() }

// ValidateWorkspaceInput validates a WorkspaceInput value. Slug is
// required and must match the slug pattern; name defaults to the slug at
// insert time so it is optional here; description is plain text.
func ValidateWorkspaceInput(in WorkspaceInput) FieldErrors {
	var errs FieldErrors
	switch {
	case in.Slug == "":
		errs = FieldErrors{"slug": required("Slug is required.")}
	case !workspaceSlugRE.MatchString(in.Slug):
		errs = FieldErrors{"slug": FieldError{Code: ErrCodeSlugPattern, Message: "Slug must match " + workspaceSlugRE.String() + "."}}
	case utf8.RuneCountInString(in.Name) > MaxTitleRunes:
		errs = FieldErrors{"name": tooLong(fmt.Sprintf("Name must be %d characters or fewer.", MaxTitleRunes), MaxTitleRunes)}
	}
	if utf8.RuneCountInString(in.Description) > MaxBodyRunes {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["description"] = tooLong(fmt.Sprintf("Description must be %d characters or fewer.", MaxBodyRunes), MaxBodyRunes)
	}
	return errs
}

// ValidateCommentBody validates a comment body; issue and discussion
// comments share the same rule. A zero FieldError means valid.
func ValidateCommentBody(body string) FieldError {
	switch {
	case body == "":
		return required("Body is required.")
	case utf8.RuneCountInString(body) > MaxBodyRunes:
		return tooLong(fmt.Sprintf("Comment must be %d characters or fewer.", MaxBodyRunes), MaxBodyRunes)
	}
	return FieldError{}
}

// ValidatePullRequestInput validates a PRInput value: title rules are the
// issue/discussion ones, and both refs are required (already normalized
// by the caller — sameness is a store error, not a field error).
func ValidatePullRequestInput(in PRInput) FieldErrors {
	errs := ValidateIssueInput(IssueInput{Title: in.Title, Body: in.Body})
	if in.BaseRef == "" {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["base"] = required("Base ref is required.")
	}
	if in.HeadRef == "" {
		if errs == nil {
			errs = FieldErrors{}
		}
		errs["head"] = required("Head ref is required.")
	}
	return errs
}
