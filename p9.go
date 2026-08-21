// Package p9 implements the wire encoding for the Plan 9 filesystem
// protocol, 9P2000, as specified in the Plan 9 manual pages intro(5)
// and fcall(3). It provides only the message types and their
// marshaling/unmarshaling to and from the wire format; connection
// handling and RPC dispatch live in the client and server packages.
package p9

const (
	// Version is the protocol version string sent in Tversion.
	Version = "9P2000"

	// VersionUnknown is the version string a server returns when it
	// does not support the version requested in Tversion.
	VersionUnknown = "unknown"

	// DefaultMsize is a reasonable default maximum message size for
	// a client or server that has no other preference.
	DefaultMsize = 8192

	// MaxWElem is the maximum number of path elements permitted in a
	// single Twalk message.
	MaxWElem = 16
)

// NoTag marks a message with no associated tag. It is only ever used
// on the first Tversion of a connection.
const NoTag Tag = 0xFFFF

// NoFid marks the absence of an authentication fid in Tattach.
const NoFid Fid = 0xFFFFFFFF
