// Package logo probes terminal bitmap support and renders the embedded
// 512x512 brand mark as a Sixel or kitty image.
//
// Probe must be called before Bubble Tea takes over stdin. Everything else is
// passive: a Capability that is not Available never emits a graphics control
// sequence, so callers can fall back to a text logo without cleaning up.
package logo
