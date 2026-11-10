// Package github lists a repository's releases and tags and fetches release bodies. With a token
// it uses batched GraphQL queries; without one it falls back to the REST API.
package github
