import { exec } from 'child_process';
import * as fs from 'fs';
import * as path from 'path';
import { promisify } from 'util';

interface ValidatorAdd {
  address: string;
  cardano_like_chains: string[]; // chain_name:multisig_verification:fee_verification:multisig_stake_verification:fee_stake_verification
  blade_bls: string;
  nexus: boolean;
}

interface GovernanceProposal {
  proposal_id: string;
  input: string;
}

interface CardanoLikeChain {
  chain_name: string;
  multisig_verification: string;
  fee_verification: string;
  multisig_stake_verification: string;
  fee_stake_verification: string;
}

function formatCardanoLikeChains(chains: CardanoLikeChain[]): string[] {
  let retVal: string[] = [];
  for (const chain of chains) {
    retVal.push(`${chain.chain_name}:${chain.multisig_verification}:${chain.fee_verification}:${chain.multisig_stake_verification}:${chain.fee_stake_verification}`);
  }
  return retVal;
}

////////////////////////////////////////////////////////////////////////////////////////////////////
// Configuration - modify these values as needed
// Define Cardano-like chains
const cardanoLikeChains: CardanoLikeChain[] = [
  {
    chain_name: 'prime',
    multisig_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    fee_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    multisig_stake_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    fee_stake_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a'
  },
  {
    chain_name: 'vector',
    multisig_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    fee_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    multisig_stake_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a',
    fee_stake_verification: 'f6b167a444402c7f42c6445d5f629f0a9b7944b29b7766ae1991b554ccdaba7a'
  }
];

// Define validators to add
const validatorsAdd: ValidatorAdd[] = [
  {
    address: '0x1234567890abcdef1234567890abcdef12345678',
    cardano_like_chains: formatCardanoLikeChains(cardanoLikeChains),
    blade_bls: '03516badf21abb14e32d2577118459f298d395f6d8ad451bb73097997d670f912c248ba7ddb029b0a440a56ed7c3c623f16c59fe19f1bbbafe311c9ed9c1f1342796f24214228f21b0ad5b1aafa94a12ed85790f873fa6229f3bee1672b4a69608050ea872196734ca37599493a1e165cf98f3ad1b6e9e390595adaa637730ef',
    nexus: true
  }
];

// Define validators to remove
const validatorsRemove: string[] = ['0x742D35CC6634C0532925A3B844BC454E4438F44E'];

// Other configurations, file path, RPC URL, validator private key, etc.
const filePath = './proposal.json';
const rpcUrl = 'http://localhost:10002';
const validatorKey = 'e5340e34447909cf64e6206a923810425e114b5534097658c0085729fd027e52';
const bladeExePath = '../blade-apex-bridge/blade';
//////////////////////////////////////////////////////////////////////////////////////////////////////////

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

const execPromise = promisify(exec);

async function executeCommand(command: string): Promise<string> {
  try {
    console.log("Running command...");
    const { stdout, stderr } = await execPromise(command); // Waits for command to complete
    if (stderr) {
      console.error('stderr:', stderr);
    }
    console.log("Command finished.");
    return stdout;
  } catch (error) {
    console.error("Error executing command:", error);
    return '';
  }
}

// Initialize, submit and vote for the VSC proposal
async function init() {
  // add validators
  let args: string;
  for (const validator of validatorsAdd) {
    const command = `${bladeExePath} proposal create-vsc-proposal add-validator`;
    let args = ` --file ${filePath} --address ${validator.address} --blade ${validator.blade_bls}`;
    for (const chain of validator.cardano_like_chains) {
      args += ` --cardano-like-chain ${chain}`;
    }
    if (validator.nexus) {
      args += ' --nexus';
    }

    await executeCommand(command + args);
  }

  // remove validators
  for (const address of validatorsRemove) {
    const command = `${bladeExePath} proposal create-vsc-proposal remove-validator`;
    args = ` --file ${filePath} --address ${address}`;

    await executeCommand(command + args);
  }

  // submit proposal
  console.log("Submitting...");
  let proposalID: string = '';
  const submitCommand = `${bladeExePath} proposal submit --path ${filePath} --json-rpc ${rpcUrl} --private-key ${validatorKey} --description "vsc"`;
  await executeCommand(submitCommand).then((output) => {
    const filePath = path.join(__dirname, 'governance_result.json');
    fs.writeFileSync(filePath, output); // Overwrites the file
    const proposal: GovernanceProposal = JSON.parse(output);
    proposalID = proposal.proposal_id;
    console.log("Proposal submitted:", output);
  });

  // wait until proposal is active
  let loop:boolean = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposalID}`).then((output) => {
      console.log("Proposal state:", output);
      if (output.includes('1')) {
        console.log("Proposal is active, proceeding to vote...");
        loop = false;
      } else {
        console.log("Proposal is not active, cannot vote.");
      }
    });
  }

  // vote proposal
  console.log("Voting...");
  const voteCommand = `${bladeExePath} proposal vote --json-rpc ${rpcUrl} --private-key ${validatorKey} --proposal-id ${proposalID}`;
  await executeCommand(voteCommand).then(() => {
    console.log("Script finished.");
  });
}

// Queue and execute the VSC proposal
async function execute() {
  // read proposal from governance_result.json
  let proposal:GovernanceProposal = {proposal_id: '', input: ''};
  const filePath = path.join(__dirname, 'governance_result.json');
  try {
    const jsonString = fs.readFileSync(filePath, 'utf-8');
    proposal = JSON.parse(jsonString);
    console.log("JSON proposal:", proposal);
  } catch (error) {
    console.error('Error reading file:', error);
    process.exit(1);
  }

  // proposal has to be in state succeded before executing (voted with quorum)
  await executeCommand(`cast call --rpc-url ${rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
    console.log("Proposal state:", output);
    if (output.includes('4')) {
      console.log("Proposal is succeded, proceeding with execution...");
    } else {
      console.error("Proposal is active, cannot proceed.");
      process.exit(1);
    }
  });

  // queue proposal
  console.log("Queueing...");
  const queueCommand = `${bladeExePath} proposal queue --json-rpc ${rpcUrl} --private-key ${validatorKey} --input ${proposal.input} --description "vsc"`;
  await executeCommand(queueCommand).then(() => {
    console.log("Proposal queued");
  });

  // wait until proposal is queued
  let loop:boolean = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
      console.log("Proposal state:", output);
      if (output.includes('5')) {
        console.log("Proposal is queued, proceeding to execute...");
        loop = false;
      } else {
        console.log("Proposal is not queued yet, waiting...");
      }
    });
  }

  // execute proposal
  console.log("Executing...");
  const execCommand = `${bladeExePath} proposal execute --json-rpc ${rpcUrl} --private-key ${validatorKey} --input ${proposal.input} --description "vsc"`;
  await executeCommand(execCommand).then(() => {
    console.log("Proposal executed");
  });

  // wait until proposal is executed
  loop = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
      console.log("Proposal state:", output);
      if (output.includes('7')) {
        console.log("Proposal is executed, VSC starting...");
        loop = false;
      } else {
        console.log("Proposal is not executed yet, waiting...");
      }
    });
  }

  // wait until VSC is started
  loop = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0xaBef000000000000000000000000000000000000 "function isNewValidatorSetPending()"`).then((output) => {
      console.log("VSC state:", output);
      if (output == 'true') {
        console.log("VSC started...");
        loop = false;
      } else {
        console.log("VSC not started yet, waiting...");
      }
    });
  }

  // wait until VSC is finished
  loop = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0xaBef000000000000000000000000000000000000 "function isNewValidatorSetPending()"`).then((output) => {
      console.log("VSC state:", output);
      if (output == 'false') {
        console.log("VSC finished...");
        loop = false;
      } else {
        console.log("VSC in progress...");
      }
    });
  }

  // check stake amount for added validators
  for (const validator of validatorsAdd) {
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0x0000000000000000000000000000000000010022 "function stakeOf(address validator)" ${validator.address}`).then((output) => {
      console.log("Validator %s stake:", validator.address, output);
      const stake = BigInt(output);
      if (stake > BigInt(0)) {
        console.log("Validator active => OK");
      } else {
        console.error("Validator inactive => ERROR");
      }
    });
  }

  // check stake amount for removed validators
  for (const validator of validatorsRemove) {
    await executeCommand(`cast call --rpc-url ${rpcUrl} 0x0000000000000000000000000000000000010022 "function stakeOf(address validator)" ${validator}`).then((output) => {
      console.log("Validator %s stake:", validator, output);
      const stake = BigInt(output);
      if (stake > BigInt(0)) {
        console.error("Validator active => ERROR");
      } else {
        console.log("Validator inactive => OK");
      }
    });
  }

  console.log("Script finished.");
}

// Main execution
const action = process.argv.slice(2);
switch (action[0]) {
  case 'init':
    init();
    break;
  case 'execute':
    execute();
    break;
  default:
    console.log(`Unknown action: ${action[0]}`);
}