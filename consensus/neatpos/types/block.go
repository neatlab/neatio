package types

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/neatlab/neatio/core/state"
	"github.com/neatlab/neatio/core/types"
	"github.com/neatlab/neatio/log"
	"github.com/neatlab/neatio/rlp"
	. "github.com/neatlib/common-go"
	"github.com/neatlib/crypto-go"
	"github.com/neatlib/merkle-go"
	"github.com/neatlib/wire-go"
)

const MaxBlockSize = 22020096

// IntermediateBlockResult represents intermediate block execute result.
type IntermediateBlockResult struct {
	Block *types.Block
	// followed by block execute result
	State    *state.StateDB
	Receipts types.Receipts
	Ops      *types.PendingOps
}

type TdmBlock struct {
	Block              *types.Block             `json:"block"`
	NcExtra            *NeatconExtra            `json:"tdmexdata"`
	TX3ProofData       []*types.TX3ProofData    `json:"tx3proofdata"`
	IntermediateResult *IntermediateBlockResult `json:"-"`
}

func MakeBlock(height uint64, chainID string, commit *Commit,
	block *types.Block, valHash []byte, epochNumber uint64, epochBytes []byte, tx3ProofData []*types.TX3ProofData, partSize int) (*TdmBlock, *PartSet) {
	NcExtra := &NeatconExtra{
		ChainID:        chainID,
		Height:         uint64(height),
		Time:           time.Now(),
		EpochNumber:    epochNumber,
		ValidatorsHash: valHash,
		SeenCommit:     commit,
		EpochBytes:     epochBytes,
	}

	tdmBlock := &TdmBlock{
		Block:        block,
		NcExtra:      NcExtra,
		TX3ProofData: tx3ProofData,
	}
	return tdmBlock, tdmBlock.MakePartSet(partSize)
}

// Basic validation that doesn't involve state data.
func (b *TdmBlock) ValidateBasic(ncExtra *NeatconExtra) error {

	if b.NcExtra.ChainID != ncExtra.ChainID {
		return errors.New(Fmt("Wrong Block.Header.ChainID. Expected %v, got %v", ncExtra.ChainID, b.NcExtra.ChainID))
	}
	if b.NcExtra.Height != ncExtra.Height+1 {
		return errors.New(Fmt("Wrong Block.Header.Height. Expected %v, got %v", ncExtra.Height+1, b.NcExtra.Height))
	}

	/*
		if !b.NcExtra.BlockID.Equals(blockID) {
			return errors.New(Fmt("Wrong Block.Header.LastBlockID.  Expected %v, got %v", blockID, b.NcExtra.BlockID))
		}
		if !bytes.Equal(b.NcExtra.SeenCommitHash, b.NcExtra.SeenCommit.Hash()) {
			return errors.New(Fmt("Wrong Block.Header.LastCommitHash.  Expected %X, got %X", b.NcExtra.SeenCommitHash, b.NcExtra.SeenCommit.Hash()))
		}
		if b.NcExtra.Height != 1 {
			if err := b.NcExtra.SeenCommit.ValidateBasic(); err != nil {
				return err
			}
		}
	*/
	return nil
}

func (b *TdmBlock) FillSeenCommitHash() {
	if b.NcExtra.SeenCommitHash == nil {
		b.NcExtra.SeenCommitHash = b.NcExtra.SeenCommit.Hash()
	}
}

// Computes and returns the block hash.
// If the block is incomplete, block hash is nil for safety.
func (b *TdmBlock) Hash() []byte {
	// fmt.Println(">>", b.Data)
	if b == nil || b.NcExtra.SeenCommit == nil {
		return nil
	}
	b.FillSeenCommitHash()
	return b.NcExtra.Hash()
}

func (b *TdmBlock) MakePartSet(partSize int) *PartSet {

	return NewPartSetFromData(b.ToBytes(), partSize)
}

func (b *TdmBlock) ToBytes() []byte {

	type TmpBlock struct {
		BlockData    []byte
		NcExtra      *NeatconExtra
		TX3ProofData []*types.TX3ProofData
	}
	//fmt.Printf("TdmBlock.toBytes 0 with block: %v\n", b)

	bs, err := rlp.EncodeToBytes(b.Block)
	if err != nil {
		log.Warnf("TdmBlock.toBytes error\n")
	}
	bb := &TmpBlock{
		BlockData:    bs,
		NcExtra:      b.NcExtra,
		TX3ProofData: b.TX3ProofData,
	}

	ret := wire.BinaryBytes(bb)
	return ret
}

func (b *TdmBlock) FromBytes(reader io.Reader) (*TdmBlock, error) {

	type TmpBlock struct {
		BlockData    []byte
		NcExtra      *NeatconExtra
		TX3ProofData []*types.TX3ProofData
	}

	//fmt.Printf("TdmBlock.FromBytes \n")

	var n int
	var err error
	bb := wire.ReadBinary(&TmpBlock{}, reader, MaxBlockSize, &n, &err).(*TmpBlock)
	if err != nil {
		log.Warnf("TdmBlock.FromBytes 0 error: %v\n", err)
		return nil, err
	}

	var block types.Block
	err = rlp.DecodeBytes(bb.BlockData, &block)
	if err != nil {
		log.Warnf("TdmBlock.FromBytes 1 error: %v\n", err)
		return nil, err
	}

	tdmBlock := &TdmBlock{
		Block:        &block,
		NcExtra:      bb.NcExtra,
		TX3ProofData: bb.TX3ProofData,
	}

	log.Debugf("TdmBlock.FromBytes 2 with: %v\n", tdmBlock)
	return tdmBlock, nil
}

// Convenience.
// A nil block never hashes to anything.
// Nothing hashes to a nil hash.
func (b *TdmBlock) HashesTo(hash []byte) bool {
	if len(hash) == 0 {
		return false
	}
	if b == nil {
		return false
	}
	return bytes.Equal(b.Hash(), hash)
}

func (b *TdmBlock) String() string {
	return b.StringIndented("")
}

func (b *TdmBlock) StringIndented(indent string) string {
	if b == nil {
		return "nil-Block"
	}

	return fmt.Sprintf(`Block{
%s  %v
%s  %v
%s  %v
%s}#%X`,
		indent, b.Block.String(),
		indent, b.NcExtra,
		indent, b.NcExtra.SeenCommit.StringIndented(indent+""),
		indent, b.Hash())
}

func (b *TdmBlock) StringShort() string {
	if b == nil {
		return "nil-Block"
	} else {
		return fmt.Sprintf("Block#%X", b.Hash())
	}
}

//-------------------------------------

// NOTE: Commit is empty for height 1, but never nil.
type Commit struct {
	// NOTE: The Precommits are in order of address to preserve the bonded ValidatorSet order.
	// Any peer with a block can gossip precommits by index with a peer without recalculating the
	// active ValidatorSet.
	BlockID BlockID `json:"blockID"`
	Height  uint64  `json:"height"`
	Round   int     `json:"round"`

	// BLS signature aggregation to be added here
	SignAggr crypto.BLSSignature `json:"SignAggr"`
	BitArray *BitArray

	// Volatile
	hash []byte
}

func (commit *Commit) Type() byte {
	return VoteTypePrecommit
}

func (commit *Commit) Size() int {
	return (int)(commit.BitArray.Size())
}

func (commit *Commit) NumCommits() int {
	return (int)(commit.BitArray.NumBitsSet())
}

func (commit *Commit) ValidateBasic() error {
	if commit.BlockID.IsZero() {
		return errors.New("Commit cannot be for nil block")
	}
	/*
		if commit.Type() != VoteTypePrecommit {
			return fmt.Errorf("Invalid commit type. Expected VoteTypePrecommit, got %v",
				precommit.Type)
		}

		// shall we validate the signature aggregation?
	*/

	return nil
}

func (commit *Commit) Hash() []byte {
	if commit.hash == nil {
		hash := merkle.SimpleHashFromBinary(*commit)
		commit.hash = hash
	}
	return commit.hash
}

func (commit *Commit) StringIndented(indent string) string {
	if commit == nil {
		return "nil-Commit"
	}
	return fmt.Sprintf(`Commit{
%s  BlockID:    %v
%s  Height:     %v
%s  Round:      %v
%s  Type:       %v
%s  BitArray:   %v
%s}#%X`,
		indent, commit.BlockID,
		indent, commit.Height,
		indent, commit.Round,
		indent, commit.Type(),
		indent, commit.BitArray.String(),
		indent, commit.hash)
}

//--------------------------------------------------------------------------------

type BlockID struct {
	Hash        []byte        `json:"hash"`
	PartsHeader PartSetHeader `json:"parts"`
}

func (blockID BlockID) IsZero() bool {
	return len(blockID.Hash) == 0 && blockID.PartsHeader.IsZero()
}

func (blockID BlockID) Equals(other BlockID) bool {
	return bytes.Equal(blockID.Hash, other.Hash) &&
		blockID.PartsHeader.Equals(other.PartsHeader)
}

func (blockID BlockID) Key() string {
	return string(blockID.Hash) + string(wire.BinaryBytes(blockID.PartsHeader))
}

func (blockID BlockID) WriteSignBytes(w io.Writer, n *int, err *error) {
	if blockID.IsZero() {
		wire.WriteTo([]byte("null"), w, n, err)
	} else {
		wire.WriteJSON(CanonicalBlockID(blockID), w, n, err)
	}

}

func (blockID BlockID) String() string {
	return fmt.Sprintf(`%X:%v`, blockID.Hash, blockID.PartsHeader)
}
