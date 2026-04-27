package conversation

import (
	"crypto/rand"
	"encoding/hex"
)

type BranchID string
type NodeID string

const MainBranch BranchID = "main"

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func NewBranchID() BranchID { return BranchID(newID("branch_")) }
func NewNodeID() NodeID     { return NodeID(newID("node_")) }
