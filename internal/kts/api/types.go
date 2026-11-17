// SPDX-License-Identifier: Apache-2.0

// Package api provides the public interface types for kts.
// Extensions that need kts functionality import this package instead of
// internal kts packages directly.
//
// Two files make up the contract:
//
//   - sender.go     — what a caller needs to SEND a request: a two-method
//     Sender plus its call options.
//   - descriptor.go — what a caller needs to DESCRIBE a service surface: slots,
//     methods, schemas and CLI presentation.
//
// Neither depends on anything. The protocol package re-exports every name, so
// code that already holds the protocol keeps working unchanged while consumers
// depend on this package alone.
package api
