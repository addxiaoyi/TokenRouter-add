package web

import (
	"context"
	"errors"
)

// public_pages.go defines public document lookup for embedded frontend delivery.
// The application supplies locale-selected legal copy through this contract, so
// the web package can render public pages without importing site persistence or
// exposing authenticated account data to the page renderer.

// ErrPublicDocumentNotFound distinguishes absent published copy from lookup
// failures, allowing the HTML endpoint to return a meaningful missing-page status.
var ErrPublicDocumentNotFound = errors.New("public document not found")

// PublicDocument carries one language version of a published legal document.
type PublicDocument struct {
	// Title, Markdown, and Locale belong to the same resolved document version.
	// An empty Locale retains the request language already selected by middleware.
	Title    string // resolved document title
	Markdown string // resolved Markdown source
	Locale   string // selected content language
}

// PublicDocumentLookup resolves published copy for an exact document ID.
// Implementations honor cancellation and the context language, returning
// ErrPublicDocumentNotFound for absent copy and other errors for failed reads.
type PublicDocumentLookup func(context.Context, string) (PublicDocument, error)

// FrontendOption configures embedded delivery before the server handles requests.
type FrontendOption func(*FrontendServer)

// WithPublicPages enables initial public content for home and legal addresses.
// Supply a callable lookup before serving requests; nil disables this option.
// Legal reads remain separate from the cached settings envelope so published
// document changes can appear in the next HTML response.
func WithPublicPages(lookup PublicDocumentLookup) FrontendOption {
	return func(server *FrontendServer) {
		server.publicDocuments = lookup
	}
}
