package conversation

import (
	"crypto/rand"
	"encoding/hex"
)

type ConversationID string
type SessionID string
type BranchID string
type NodeID string

const MainBranch BranchID = "main"

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func NewConversationID() ConversationID { return ConversationID(newID("conv_")) }
func NewSessionID() SessionID           { return SessionID(newID("sess_")) }
func NewBranchID() BranchID             { return BranchID(newID("branch_")) }
func NewNodeID() NodeID                 { return NodeID(newID("node_")) }
