package crypto

/*

This part of the package wraps crypto operations needed elsewhere

In particular, it exposes key generation for ed25519 and nacl box

It also defines NodeID and TreeID as hashes of keys, and wraps hash functions

*/

import (
	"crypto/rand"
	"crypto/sha512"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/box"

	"github.com/yggdrasil-network/yggdrasil-go/src/util"
)

////////////////////////////////////////////////////////////////////////////////

// NodeID and TreeID

const NodeIDLen = sha512.Size
const TreeIDLen = sha512.Size
const handleLen = 8

type NodeID [NodeIDLen]byte
type TreeID [TreeIDLen]byte
type Handle [handleLen]byte

func GetNodeID(pub *BoxPubKey) *NodeID {
	h := sha512.Sum512(pub[:])
	return (*NodeID)(&h)
}

func GetTreeID(pub *SigPubKey) *TreeID {
	h := sha512.Sum512(pub[:])
	return (*TreeID)(&h)
}

func NewHandle() *Handle {
	var h Handle
	_, err := rand.Read(h[:])
	if err != nil {
		panic(err)
	}
	return &h
}

////////////////////////////////////////////////////////////////////////////////

// Signatures

const SigPubKeyLen = ed25519.PublicKeySize
const SigPrivKeyLen = ed25519.PrivateKeySize
const SigLen = ed25519.SignatureSize

type SigPubKey [SigPubKeyLen]byte
type SigPrivKey [SigPrivKeyLen]byte
type SigBytes [SigLen]byte

func NewSigKeys() (*SigPubKey, *SigPrivKey) {
	var pub SigPubKey
	var priv SigPrivKey
	pubSlice, privSlice, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	copy(pub[:], pubSlice)
	copy(priv[:], privSlice)
	return &pub, &priv
}

func Sign(priv *SigPrivKey, msg []byte) *SigBytes {
	var sig SigBytes
	sigSlice := ed25519.Sign(priv[:], msg)
	copy(sig[:], sigSlice)
	return &sig
}

func Verify(pub *SigPubKey, msg []byte, sig *SigBytes) bool {
	// Should sig be an array instead of a slice?...
	// It's fixed size, but
	return ed25519.Verify(pub[:], msg, sig[:])
}

////////////////////////////////////////////////////////////////////////////////

// NaCl-like crypto "box" (curve25519+xsalsa20+poly1305)

const BoxPubKeyLen = 32
const BoxPrivKeyLen = 32
const BoxSharedKeyLen = 32
const BoxNonceLen = 24
const BoxOverhead = box.Overhead

type BoxPubKey [BoxPubKeyLen]byte
type BoxPrivKey [BoxPrivKeyLen]byte
type BoxSharedKey [BoxSharedKeyLen]byte
type BoxNonce [BoxNonceLen]byte

func NewBoxKeys() (*BoxPubKey, *BoxPrivKey) {
	pubBytes, privBytes, err := box.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	pub := (*BoxPubKey)(pubBytes)
	priv := (*BoxPrivKey)(privBytes)
	return pub, priv
}

func GetSharedKey(myPrivKey *BoxPrivKey,
	othersPubKey *BoxPubKey) *BoxSharedKey {
	var shared [BoxSharedKeyLen]byte
	priv := (*[BoxPrivKeyLen]byte)(myPrivKey)
	pub := (*[BoxPubKeyLen]byte)(othersPubKey)
	box.Precompute(&shared, pub, priv)
	return (*BoxSharedKey)(&shared)
}

func BoxOpen(shared *BoxSharedKey,
	boxed []byte,
	nonce *BoxNonce) ([]byte, bool) {
	out := util.GetBytes()
	s := (*[BoxSharedKeyLen]byte)(shared)
	n := (*[BoxNonceLen]byte)(nonce)
	unboxed, success := box.OpenAfterPrecomputation(out, boxed, n, s)
	return unboxed, success
}

func BoxSeal(shared *BoxSharedKey, unboxed []byte, nonce *BoxNonce) ([]byte, *BoxNonce) {
	if nonce == nil {
		nonce = NewBoxNonce()
	}
	nonce.Increment()
	out := util.GetBytes()
	s := (*[BoxSharedKeyLen]byte)(shared)
	n := (*[BoxNonceLen]byte)(nonce)
	boxed := box.SealAfterPrecomputation(out, unboxed, n, s)
	return boxed, nonce
}

func NewBoxNonce() *BoxNonce {
	var nonce BoxNonce
	_, err := rand.Read(nonce[:])
	for ; err == nil && nonce[0] == 0xff; _, err = rand.Read(nonce[:]) {
		// Make sure nonce isn't too high
		// This is just to make rollover unlikely to happen
		// Rollover is fine, but it may kill the session and force it to reopen
	}
	if err != nil {
		panic(err)
	}
	return &nonce
}

func (n *BoxNonce) Increment() {
	oldNonce := *n
	n[len(n)-1] += 2
	for i := len(n) - 2; i >= 0; i-- {
		if n[i+1] < oldNonce[i+1] {
			n[i] += 1
		}
	}
}

// Used to subtract one nonce from another, staying in the range +- 64.
// This is used by the nonce progression machinery to advance the bitmask of recently received packets (indexed by nonce), or to check the appropriate bit of the bitmask.
// It's basically part of the machinery that prevents replays and duplicate packets.
func (n *BoxNonce) Minus(m *BoxNonce) int64 {
	diff := int64(0)
	for idx := range n {
		diff *= 256
		diff += int64(n[idx]) - int64(m[idx])
		if diff > 64 {
			diff = 64
		}
		if diff < -64 {
			diff = -64
		}
	}
	return diff
}
