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

interface Config {
  validatorsRemove: string[];
  validatorsAdd: ValidatorAdd[],
  proposalFilePath: string;
  proposalDesc: string;
  rpcUrl: string;
  validatorPk: string;
  bladeBinPath: string;
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

const loadConfig = (configPath: string): Config | undefined => {
  try {
    const jsonString = fs.readFileSync(configPath, 'utf-8');
    const config = JSON.parse(jsonString);
    console.log(`config - ${configPath}:`, JSON.stringify(config, undefined, '  '));

    for (let i = 0; i < config['validatorsAdd'].length; ++i) {
      config['validatorsAdd'][i]['cardano_like_chains'] = formatCardanoLikeChains(config['validatorsAdd'][i]['cardano_like_chains']);
    }

    return config as Config;
  } catch (error) {
    console.error('Error reading file:', error);
    process.exit(1);
  }
}


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
async function init(config: Config) {
  // add validators
  let args: string;
  for (const validator of config.validatorsAdd) {
    const command = `${config.bladeBinPath} proposal create-vsc-proposal add-validator`;
    let args = ` --file ${config.proposalFilePath} --address ${validator.address} --blade ${validator.blade_bls}`;
    for (const chain of validator.cardano_like_chains) {
      args += ` --cardano-like-chain ${chain}`;
    }
    if (validator.nexus) {
      args += ' --nexus';
    }

    await executeCommand(command + args);
  }

  // remove validators
  for (const address of config.validatorsRemove) {
    const command = `${config.bladeBinPath} proposal create-vsc-proposal remove-validator`;
    args = ` --file ${config.proposalFilePath} --address ${address}`;

    await executeCommand(command + args);
  }

  // submit proposal
  console.log("Submitting...");
  let proposalID: string = '';
  const submitCommand = `${config.bladeBinPath} proposal submit --path ${config.proposalFilePath} --json-rpc ${config.rpcUrl} --private-key ${config.validatorPk} --description "${config.proposalDesc}"`;
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
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposalID}`).then((output) => {
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
  const voteCommand = `${config.bladeBinPath} proposal vote --json-rpc ${config.rpcUrl} --private-key ${config.validatorPk} --proposal-id ${proposalID}`;
  await executeCommand(voteCommand).then(() => {
    console.log("Script finished.");
  });
}

// Queue and execute the VSC proposal
async function execute(config: Config) {
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
  await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
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
  const queueCommand = `${config.bladeBinPath} proposal queue --json-rpc ${config.rpcUrl} --private-key ${config.validatorPk} --input ${proposal.input} --description "${config.proposalDesc}"`;
  await executeCommand(queueCommand).then(() => {
    console.log("Proposal queued");
  });

  // wait until proposal is queued
  let loop:boolean = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
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
  const execCommand = `${config.bladeBinPath} proposal execute --json-rpc ${config.rpcUrl} --private-key ${config.validatorPk} --input ${proposal.input} --description "${config.proposalDesc}"`;
  await executeCommand(execCommand).then(() => {
    console.log("Proposal executed");
  });

  // wait until proposal is executed
  loop = true;
  while (loop) {
    await sleep(10000); // wait for 10 seconds before checking again
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x000000000000000000000000000000000000100C "function state(uint256 proposal_id)" ${proposal.proposal_id}`).then((output) => {
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
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0xaBef000000000000000000000000000000000000 "function isNewValidatorSetPending()"`).then((output) => {
      console.log("VSC state:", output);
      if (output.includes('1')) {
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
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0xaBef000000000000000000000000000000000000 "function isNewValidatorSetPending()"`).then((output) => {
      console.log("VSC state:", output);
      if (!output.includes('1')) {
        console.log("VSC finished...");
        loop = false;
      } else {
        console.log("VSC in progress...");
      }
    });
  }

  // check stake amount for added validators
  for (const validator of config.validatorsAdd) {
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x0000000000000000000000000000000000010022 "function stakeOf(address validator)" ${validator.address}`).then((output) => {
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
  for (const validator of config.validatorsRemove) {
    await executeCommand(`cast call --rpc-url ${config.rpcUrl} 0x0000000000000000000000000000000000010022 "function stakeOf(address validator)" ${validator}`).then((output) => {
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

let configPath = './vsc_config.json';
if (action.length > 1) {
  configPath = action[1];
}

const config = loadConfig(configPath);
if (config) {
  switch (action[0]) {
    case 'init':
      init(config);
      break;
    case 'execute':
      execute(config);
      break;
    default:
      console.log(`Unknown action: ${action[0]}`);
  }
}