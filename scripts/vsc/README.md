# Manual

## Execution steps
1. Navigate to this folder (vsc) and execute `npm install`
2. Execute `npx tsc ./vsc.ts`
3. Set configuration parameters (validators for add and remove, file path and name for proposal json, etc..) into `vsc_config.json`
4. Copy created js file `./vsc.js` and `./vsc_config.json` to validator node in test or prod environment
5. Disable apex bridge web
6. Execute `node vsc.js init ./vsc_config.json` That command will create proposal json file, submit proposal to governance, vote for the proposal and create `governance_proposal.json` in the same folder where `vsc.js` is placed.
7. Get `proposal_id` from the `governance_proposal.json` and send it to other validators for vote with the command:
   ```
   ./blade proposal vote
   		--json-rpc <value> // Blade node JSON-RPC address
   		--private-key <value> // validator private ECDSA key
   		--proposal-id <value> // VSC governace proposal ID
    ```
8. After voting is completed with the quorum and after governance `votingPeriod` is expired (currently set to 10000 blocks) we can proceed with the 2nd part of procedure.
9. Execute `node vsc.js execute ./vsc_config.json` That command will do proposal queue and execute and wait for VSC to start and to finish. It will also check stake amount of added and removed validators at the end.
10. Restart apex-bridge process on every node.
11. Enable apex bridge web