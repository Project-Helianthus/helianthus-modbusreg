// Package profiles owns the registry's committed profile data that product
// code embeds. It holds data only: it decodes nothing, decides nothing and
// imports no module package.
//
// The directory also holds the committed vendor data under vendors/; only the
// files this package embeds reach product code through it.
package profiles
