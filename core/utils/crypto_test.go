// Copyright 2018 The dexon-consensus Authors
// This file is part of the dexon-consensus library.
//
// The dexon-consensus library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus library. If not, see
// <http://www.gnu.org/licenses/>.

package utils

import (
	"testing"
	"time"

	"github.com/dexon-foundation/dexon-consensus/common"
	"github.com/dexon-foundation/dexon-consensus/core/crypto"
	"github.com/dexon-foundation/dexon-consensus/core/crypto/dkg"
	"github.com/dexon-foundation/dexon-consensus/core/crypto/ecdsa"
	"github.com/dexon-foundation/dexon-consensus/core/types"
	typesDKG "github.com/dexon-foundation/dexon-consensus/core/types/dkg"
	"github.com/stretchr/testify/suite"
)

type CryptoTestSuite struct {
	suite.Suite
}

var myNID = types.NodeID{Hash: common.NewRandomHash()}

func (s *CryptoTestSuite) prepareBlock(prevBlock *types.Block) *types.Block {
	now := time.Now().UTC()
	if prevBlock == nil {
		return &types.Block{
			Timestamp: now,
			Finalization: types.FinalizationResult{
				Timestamp: time.Now(),
				Height:    0,
			},
		}
	}
	s.Require().NotEqual(prevBlock.Hash, common.Hash{})
	return &types.Block{
		ParentHash: prevBlock.Hash,
		Timestamp:  now,
		Position: types.Position{
			Height: prevBlock.Position.Height + 1,
		},
		Finalization: types.FinalizationResult{
			Timestamp: time.Now(),
			Height:    prevBlock.Finalization.Height + 1,
		},
	}
}

func (s *CryptoTestSuite) newBlock(prevBlock *types.Block) *types.Block {
	block := s.prepareBlock(prevBlock)
	var err error
	block.Hash, err = HashBlock(block)
	s.Require().NoError(err)
	return block
}

func (s *CryptoTestSuite) generateCompactionChain(
	length int, prv crypto.PrivateKey) []*types.Block {
	blocks := make([]*types.Block, length)
	var prevBlock *types.Block
	for idx := range blocks {
		block := s.newBlock(prevBlock)
		prevBlock = block
		blocks[idx] = block
	}
	return blocks
}

func (s *CryptoTestSuite) generateBlockChain(
	length int, signer *Signer) []*types.Block {
	blocks := make([]*types.Block, length)
	var prevBlock *types.Block
	for idx := range blocks {
		block := s.newBlock(prevBlock)
		blocks[idx] = block
		err := signer.SignBlock(block)
		s.Require().NoError(err)
	}
	return blocks
}

func (s *CryptoTestSuite) TestBlockSignature() {
	prv, err := ecdsa.NewPrivateKey()
	s.Require().NoError(err)
	blocks := s.generateBlockChain(10, NewSigner(prv))
	blockMap := make(map[common.Hash]*types.Block)
	for _, block := range blocks {
		blockMap[block.Hash] = block
	}
	for _, block := range blocks {
		if !block.IsGenesis() {
			parentBlock, exist := blockMap[block.ParentHash]
			s.Require().True(exist)
			s.True(parentBlock.Position.Height == block.Position.Height-1)
			hash, err := HashBlock(parentBlock)
			s.Require().NoError(err)
			s.Equal(hash, block.ParentHash)
		}
		s.NoError(VerifyBlockSignature(block))
	}
}

func (s *CryptoTestSuite) TestVoteSignature() {
	prv, err := ecdsa.NewPrivateKey()
	s.Require().NoError(err)
	pub := prv.PublicKey()
	nID := types.NewNodeID(pub)
	vote := types.NewVote(types.VoteInit, common.NewRandomHash(), 1)
	vote.ProposerID = nID
	vote.Signature, err = prv.Sign(HashVote(vote))
	s.Require().NoError(err)
	ok, err := VerifyVoteSignature(vote)
	s.Require().NoError(err)
	s.True(ok)
	vote.Type = types.VoteCom
	ok, err = VerifyVoteSignature(vote)
	s.Require().NoError(err)
	s.False(ok)
}

func (s *CryptoTestSuite) TestCRSSignature() {
	crs := common.NewRandomHash()
	prv, err := ecdsa.NewPrivateKey()
	s.Require().NoError(err)
	pub := prv.PublicKey()
	nID := types.NewNodeID(pub)
	block := &types.Block{
		ProposerID: nID,
	}
	block.CRSSignature, err = prv.Sign(hashCRS(block, crs))
	s.Require().NoError(err)
	ok, err := VerifyCRSSignature(block, crs)
	s.Require().NoError(err)
	s.True(ok)
	block.Position.Height++
	ok, err = VerifyCRSSignature(block, crs)
	s.Require().NoError(err)
	s.False(ok)
}

func (s *CryptoTestSuite) TestDKGSignature() {
	prv, err := ecdsa.NewPrivateKey()
	s.Require().NoError(err)
	nID := types.NewNodeID(prv.PublicKey())
	prvShare := &typesDKG.PrivateShare{
		ProposerID:   nID,
		Round:        5,
		PrivateShare: *dkg.NewPrivateKey(),
	}
	prvShare.Signature, err = prv.Sign(hashDKGPrivateShare(prvShare))
	s.Require().NoError(err)
	ok, err := VerifyDKGPrivateShareSignature(prvShare)
	s.Require().NoError(err)
	s.True(ok)
	prvShare.Round++
	ok, err = VerifyDKGPrivateShareSignature(prvShare)
	s.Require().NoError(err)
	s.False(ok)

	id := dkg.NewID([]byte{13})
	_, pkShare := dkg.NewPrivateKeyShares(1)
	mpk := &typesDKG.MasterPublicKey{
		ProposerID:      nID,
		Round:           5,
		DKGID:           id,
		PublicKeyShares: *pkShare,
	}
	mpk.Signature, err = prv.Sign(hashDKGMasterPublicKey(mpk))
	s.Require().NoError(err)
	ok, err = VerifyDKGMasterPublicKeySignature(mpk)
	s.Require().NoError(err)
	s.True(ok)
	mpk.Round++
	ok, err = VerifyDKGMasterPublicKeySignature(mpk)
	s.Require().NoError(err)
	s.False(ok)

	prvShare.Round = 5
	prvShare.Signature, err = prv.Sign(hashDKGPrivateShare(prvShare))
	s.Require().NoError(err)
	complaint := &typesDKG.Complaint{
		ProposerID:   nID,
		Round:        5,
		PrivateShare: *prvShare,
	}
	complaint.Signature, err = prv.Sign(hashDKGComplaint(complaint))
	s.Require().NoError(err)
	ok, err = VerifyDKGComplaintSignature(complaint)
	s.Require().NoError(err)
	s.True(ok)
	// Test incorrect complaint signature.
	complaint.Round++
	ok, err = VerifyDKGComplaintSignature(complaint)
	s.Require().NoError(err)
	s.False(ok)
	// Test mismatch round.
	complaint.Round--
	complaint.PrivateShare.Round++
	complaint.Signature, err = prv.Sign(hashDKGComplaint(complaint))
	s.Require().NoError(err)
	ok, err = VerifyDKGComplaintSignature(complaint)
	s.Require().NoError(err)
	s.False(ok)
	// Test incorrect private share signature.
	complaint.PrivateShare.Round--
	complaint.PrivateShare.ReceiverID = types.NodeID{Hash: common.NewRandomHash()}
	complaint.Signature, err = prv.Sign(hashDKGComplaint(complaint))
	s.Require().NoError(err)
	ok, err = VerifyDKGComplaintSignature(complaint)
	s.Require().NoError(err)
	s.False(ok)

	sig := &typesDKG.PartialSignature{
		ProposerID:       nID,
		Round:            5,
		PartialSignature: dkg.PartialSignature{},
	}
	sig.Signature, err = prv.Sign(hashDKGPartialSignature(sig))
	s.Require().NoError(err)
	ok, err = VerifyDKGPartialSignatureSignature(sig)
	s.Require().NoError(err)
	s.True(ok)
	sig.Round++
	ok, err = VerifyDKGPartialSignatureSignature(sig)
	s.Require().NoError(err)
	s.False(ok)

	ready := &typesDKG.MPKReady{
		ProposerID: nID,
		Round:      5,
	}
	ready.Signature, err = prv.Sign(hashDKGMPKReady(ready))
	s.Require().NoError(err)
	ok, err = VerifyDKGMPKReadySignature(ready)
	s.Require().NoError(err)
	s.True(ok)
	ready.Round++
	ok, err = VerifyDKGMPKReadySignature(ready)
	s.Require().NoError(err)
	s.False(ok)

	final := &typesDKG.Finalize{
		ProposerID: nID,
		Round:      5,
	}
	final.Signature, err = prv.Sign(hashDKGFinalize(final))
	s.Require().NoError(err)
	ok, err = VerifyDKGFinalizeSignature(final)
	s.Require().NoError(err)
	s.True(ok)
	final.Round++
	ok, err = VerifyDKGFinalizeSignature(final)
	s.Require().NoError(err)
	s.False(ok)
}

func TestCrypto(t *testing.T) {
	suite.Run(t, new(CryptoTestSuite))
}
