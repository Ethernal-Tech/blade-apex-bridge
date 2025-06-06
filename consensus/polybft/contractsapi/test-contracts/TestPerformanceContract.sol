// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.24;

// import "hardhat/console.sol";
// import "@openzeppelin/contracts/utils/Strings.sol";

contract TestPerformanceContract {
    struct SignedBatch {
        uint256 batchID;
        uint256 counter; // any data, can be used to simulate batches without quorum (which will stay in storage)
        uint256 validatorID; // instead of addresss; easier to write tests
        bytes signature; // can be anything, sc will not check
    }

    struct ConfirmedBatch {
        uint256 batchID;
        uint256 bitmap;
        uint256 counter;
        bytes[] signatures;
    }

    // hash -> multisig / bls signatures
    mapping(bytes32 => bytes[]) private signatures;

    mapping(bytes32 => uint256) private bitmap;

    // hash -> validator ID (uint8) -> true/false
    mapping(bytes32 => mapping(uint256 => bool)) private hasVoted; // for resubmit

    uint256 private hashesCount;

    ConfirmedBatch[] private confirmedBatches;

    uint256 private lastBatchID;

    uint256 private quorumCnt;

    bool private checkBatchID;

    bool private deleteTemporaryMappingsAfterQuorum;

    constructor(
        uint256 _quorumCnt,
        bool _checkBatchID,
        bool _deleteTemporaryMappingsAfterQuorum
    ) {
        quorumCnt = _quorumCnt;
        checkBatchID = _checkBatchID;
        deleteTemporaryMappingsAfterQuorum = _deleteTemporaryMappingsAfterQuorum;
        lastObservedBlock[1] = CardanoBlock(0, 0x0000000000000000000000000000000000000000000000000000000000000000);
    }

    function submitSignedBatch(SignedBatch calldata _signedBatch) external {
        // console.log("Batch has been submitted!");
        // console.log(Strings.toString(_signedBatch.validatorID));
        // console.log(Strings.toString(_signedBatch.batchID));
        // console.log(Strings.toString(_signedBatch.counter));

        uint256 _validatorID = _signedBatch.validatorID;
        uint256 _batchID = _signedBatch.batchID;
        uint256 _counter = _signedBatch.counter;

        if (checkBatchID && lastBatchID + 1 != _batchID) {
            // console.log("Invalid batch ID!");
            // console.log(Strings.toString(lastBatchID + 1));
            return;
        }

        bytes32 _sbHash = keccak256(abi.encodePacked(_batchID, _counter));

        // check if caller already voted for same hash
        if (hasVoted[_sbHash][_validatorID]) {
            // console.log("Validator already voted!");
            return;
        }

        uint256 _numberOfVotes = signatures[_sbHash].length;

        if (_numberOfVotes == quorumCnt) {
            // console.log("Quorum is already reached");
            return;
        }

        hasVoted[_sbHash][_validatorID] = true;

        // increment hashes count if this hash is not seen before
        if (_numberOfVotes == 0) {
            // console.log("New hash has been created");
            hashesCount++;
        }

        signatures[_sbHash].push(_signedBatch.signature);
        unchecked {
            bitmap[_sbHash] = bitmap[_sbHash] | (1 << _validatorID);
        }

        // check if quorum reached (+1 is last vote)
        if (_numberOfVotes + 1 >= quorumCnt) {
            // console.log("Quorum has been reached!");
            confirmedBatches.push(
                ConfirmedBatch(
                    _batchID,
                    bitmap[_sbHash],
                    _counter,
                    signatures[_sbHash]
                )
            );

            if (lastBatchID < _batchID) {
                lastBatchID = _batchID;
            }

            if (deleteTemporaryMappingsAfterQuorum) {
                // remove from storage but for that exactly hash
                delete signatures[_sbHash];
                delete bitmap[_sbHash];
            }
        }
    }

    function getConfirmedBatches()
        external
        view
        returns (ConfirmedBatch[] memory)
    {
        ConfirmedBatch[] memory batches = new ConfirmedBatch[](
            confirmedBatches.length
        );

        for (uint i = 0; i < confirmedBatches.length; i++) {
            batches[i] = confirmedBatches[i];
        }

        return batches;
    }

    function getHashesCount() external view returns (uint256) {
        return hashesCount;
    }

    function getLastBatchID() external view returns (uint256) {
        return lastBatchID;
    }

    struct CardanoBlock {
        uint256 blockSlot;
        bytes32 blockHash;
    }

    mapping(uint8 => CardanoBlock) private lastObservedBlock;
    mapping(bytes32 => mapping(address => bool)) private validatorVote;
    mapping(bytes32 => uint8) private votes;

    function updateBlocks(uint8 _chainId, CardanoBlock[] calldata _blocks, address _caller, uint8 _index) public {
        // Check if the caller has already voted for this claim
        uint256 _quorumCnt = quorumCnt;
        uint8 _validatorIdx = _index;
        uint256 _blocksLength = _blocks.length;
        CardanoBlock memory _lastObservedBlock = lastObservedBlock[_chainId];
        uint256 _bitmapValue;
        uint256 _bitmapNewValue;
        bytes32 _chash;
        uint256 _votesNum;

        for (uint i; i < _blocksLength; ++i) {
            CardanoBlock calldata _cblock = _blocks[i];
            if (_cblock.blockSlot <= _lastObservedBlock.blockSlot) {
                continue;
            }

            _chash = keccak256(abi.encodePacked(_chainId, _cblock.blockHash, _cblock.blockSlot));
            _bitmapValue = bitmap[_chash];
            unchecked {
                _bitmapNewValue = _bitmapValue | (1 << _validatorIdx);
            }

            // check if caller already voted for same hash and skip if he did
            if (_bitmapValue == _bitmapNewValue) {
                continue;
            }

            bitmap[_chash] = _bitmapNewValue;

            // Brian Kernighan's algorithm
            // @see https://github.com/estarriolvetch/solidity-bits/blob/main/contracts/Popcount.sol
            unchecked {
                for (_votesNum = 0; _bitmapNewValue != 0; _votesNum++) {
                    _bitmapNewValue &= _bitmapNewValue - 1;
                }
            }

            if (_votesNum >= _quorumCnt) {
                _lastObservedBlock = _cblock;
                // can delete because of check
                //  if (_cblock.blockSlot <= _lastObservedBlock.blockSlot)
                delete bitmap[_chash];
            }
        }

        lastObservedBlock[_chainId] = _lastObservedBlock;
    }

    function getLastObservedBlock(uint8 _chainId) external view returns (CardanoBlock memory _cb) {
        return lastObservedBlock[_chainId];
    }
}
