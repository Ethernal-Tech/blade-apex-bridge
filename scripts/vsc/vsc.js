"use strict";
var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
var __generator = (this && this.__generator) || function (thisArg, body) {
    var _ = { label: 0, sent: function() { if (t[0] & 1) throw t[1]; return t[1]; }, trys: [], ops: [] }, f, y, t, g = Object.create((typeof Iterator === "function" ? Iterator : Object).prototype);
    return g.next = verb(0), g["throw"] = verb(1), g["return"] = verb(2), typeof Symbol === "function" && (g[Symbol.iterator] = function() { return this; }), g;
    function verb(n) { return function (v) { return step([n, v]); }; }
    function step(op) {
        if (f) throw new TypeError("Generator is already executing.");
        while (g && (g = 0, op[0] && (_ = 0)), _) try {
            if (f = 1, y && (t = op[0] & 2 ? y["return"] : op[0] ? y["throw"] || ((t = y["return"]) && t.call(y), 0) : y.next) && !(t = t.call(y, op[1])).done) return t;
            if (y = 0, t) op = [op[0] & 2, t.value];
            switch (op[0]) {
                case 0: case 1: t = op; break;
                case 4: _.label++; return { value: op[1], done: false };
                case 5: _.label++; y = op[1]; op = [0]; continue;
                case 7: op = _.ops.pop(); _.trys.pop(); continue;
                default:
                    if (!(t = _.trys, t = t.length > 0 && t[t.length - 1]) && (op[0] === 6 || op[0] === 2)) { _ = 0; continue; }
                    if (op[0] === 3 && (!t || (op[1] > t[0] && op[1] < t[3]))) { _.label = op[1]; break; }
                    if (op[0] === 6 && _.label < t[1]) { _.label = t[1]; t = op; break; }
                    if (t && _.label < t[2]) { _.label = t[2]; _.ops.push(op); break; }
                    if (t[2]) _.ops.pop();
                    _.trys.pop(); continue;
            }
            op = body.call(thisArg, _);
        } catch (e) { op = [6, e]; y = 0; } finally { f = t = 0; }
        if (op[0] & 5) throw op[1]; return { value: op[0] ? op[1] : void 0, done: true };
    }
};
Object.defineProperty(exports, "__esModule", { value: true });
var child_process_1 = require("child_process");
var fs = require("fs");
var path = require("path");
var util_1 = require("util");
function formatCardanoLikeChains(chains) {
    var retVal = [];
    for (var _i = 0, chains_1 = chains; _i < chains_1.length; _i++) {
        var chain = chains_1[_i];
        retVal.push("".concat(chain.chain_name, ":").concat(chain.multisig_verification, ":").concat(chain.fee_verification, ":").concat(chain.multisig_stake_verification, ":").concat(chain.fee_stake_verification));
    }
    return retVal;
}
var loadConfig = function (configPath) {
    try {
        var jsonString = fs.readFileSync(configPath, 'utf-8');
        var config_1 = JSON.parse(jsonString);
        console.log("config - ".concat(configPath, ":"), JSON.stringify(config_1, undefined, '  '));
        for (var i = 0; i < config_1['validatorsAdd'].length; ++i) {
            config_1['validatorsAdd'][i]['cardano_like_chains'] = formatCardanoLikeChains(config_1['validatorsAdd'][i]['cardano_like_chains']);
        }
        return config_1;
    }
    catch (error) {
        console.error('Error reading file:', error);
        process.exit(1);
    }
};
//////////////////////////////////////////////////////////////////////////////////////////////////////////
function sleep(ms) {
    return new Promise(function (resolve) { return setTimeout(resolve, ms); });
}
var execPromise = (0, util_1.promisify)(child_process_1.exec);
function executeCommand(command) {
    return __awaiter(this, void 0, void 0, function () {
        var _a, stdout, stderr, error_1;
        return __generator(this, function (_b) {
            switch (_b.label) {
                case 0:
                    _b.trys.push([0, 2, , 3]);
                    console.log("Running command...");
                    return [4 /*yield*/, execPromise(command)];
                case 1:
                    _a = _b.sent(), stdout = _a.stdout, stderr = _a.stderr;
                    if (stderr) {
                        console.error('stderr:', stderr);
                    }
                    console.log("Command finished.");
                    return [2 /*return*/, stdout];
                case 2:
                    error_1 = _b.sent();
                    console.error("Error executing command:", error_1);
                    return [2 /*return*/, ''];
                case 3: return [2 /*return*/];
            }
        });
    });
}
// Initialize, submit and vote for the VSC proposal
function init(config) {
    return __awaiter(this, void 0, void 0, function () {
        var args, _i, _a, validator, command, args_1, _b, _c, chain, _d, _e, address, command, proposalID, submitCommand, loop, voteCommand;
        return __generator(this, function (_f) {
            switch (_f.label) {
                case 0:
                    _i = 0, _a = config.validatorsAdd;
                    _f.label = 1;
                case 1:
                    if (!(_i < _a.length)) return [3 /*break*/, 4];
                    validator = _a[_i];
                    command = "".concat(config.bladeBinPath, " proposal create-vsc-proposal add-validator");
                    args_1 = " --file ".concat(config.proposalFilePath, " --address ").concat(validator.address, " --blade ").concat(validator.blade_bls);
                    for (_b = 0, _c = validator.cardano_like_chains; _b < _c.length; _b++) {
                        chain = _c[_b];
                        args_1 += " --cardano-like-chain ".concat(chain);
                    }
                    if (validator.nexus) {
                        args_1 += ' --nexus';
                    }
                    return [4 /*yield*/, executeCommand(command + args_1)];
                case 2:
                    _f.sent();
                    _f.label = 3;
                case 3:
                    _i++;
                    return [3 /*break*/, 1];
                case 4:
                    _d = 0, _e = config.validatorsRemove;
                    _f.label = 5;
                case 5:
                    if (!(_d < _e.length)) return [3 /*break*/, 8];
                    address = _e[_d];
                    command = "".concat(config.bladeBinPath, " proposal create-vsc-proposal remove-validator");
                    args = " --file ".concat(config.proposalFilePath, " --address ").concat(address);
                    return [4 /*yield*/, executeCommand(command + args)];
                case 6:
                    _f.sent();
                    _f.label = 7;
                case 7:
                    _d++;
                    return [3 /*break*/, 5];
                case 8:
                    // submit proposal
                    console.log("Submitting...");
                    proposalID = '';
                    submitCommand = "".concat(config.bladeBinPath, " proposal submit --path ").concat(config.proposalFilePath, " --json-rpc ").concat(config.rpcUrl, " --private-key ").concat(config.validatorPk, " --description \"").concat(config.proposalDesc, "\"");
                    return [4 /*yield*/, executeCommand(submitCommand).then(function (output) {
                            var filePath = path.join(__dirname, 'governance_result.json');
                            fs.writeFileSync(filePath, output); // Overwrites the file
                            var proposal = JSON.parse(output);
                            proposalID = proposal.proposal_id;
                            console.log("Proposal submitted:", output);
                        })];
                case 9:
                    _f.sent();
                    loop = true;
                    _f.label = 10;
                case 10:
                    if (!loop) return [3 /*break*/, 13];
                    return [4 /*yield*/, sleep(10000)];
                case 11:
                    _f.sent(); // wait for 10 seconds before checking again
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x000000000000000000000000000000000000100C \"function state(uint256 proposal_id)\" ").concat(proposalID)).then(function (output) {
                            console.log("Proposal state:", output);
                            if (output.includes('1')) {
                                console.log("Proposal is active, proceeding to vote...");
                                loop = false;
                            }
                            else {
                                console.log("Proposal is not active, cannot vote.");
                            }
                        })];
                case 12:
                    _f.sent();
                    return [3 /*break*/, 10];
                case 13:
                    // vote proposal
                    console.log("Voting...");
                    voteCommand = "".concat(config.bladeBinPath, " proposal vote --json-rpc ").concat(config.rpcUrl, " --private-key ").concat(config.validatorPk, " --proposal-id ").concat(proposalID);
                    return [4 /*yield*/, executeCommand(voteCommand).then(function () {
                            console.log("Script finished.");
                        })];
                case 14:
                    _f.sent();
                    return [2 /*return*/];
            }
        });
    });
}
// Queue and execute the VSC proposal
function execute(config) {
    return __awaiter(this, void 0, void 0, function () {
        var proposal, filePath, jsonString, queueCommand, loop, execCommand, _loop_1, _i, _a, validator, _loop_2, _b, _c, validator;
        return __generator(this, function (_d) {
            switch (_d.label) {
                case 0:
                    proposal = { proposal_id: '', input: '' };
                    filePath = path.join(__dirname, 'governance_result.json');
                    try {
                        jsonString = fs.readFileSync(filePath, 'utf-8');
                        proposal = JSON.parse(jsonString);
                        console.log("JSON proposal:", proposal);
                    }
                    catch (error) {
                        console.error('Error reading file:', error);
                        process.exit(1);
                    }
                    // proposal has to be in state succeded before executing (voted with quorum)
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x000000000000000000000000000000000000100C \"function state(uint256 proposal_id)\" ").concat(proposal.proposal_id)).then(function (output) {
                            console.log("Proposal state:", output);
                            if (output.includes('4')) {
                                console.log("Proposal is succeded, proceeding with execution...");
                            }
                            else {
                                console.error("Proposal is active, cannot proceed.");
                                process.exit(1);
                            }
                        })];
                case 1:
                    // proposal has to be in state succeded before executing (voted with quorum)
                    _d.sent();
                    // queue proposal
                    console.log("Queueing...");
                    queueCommand = "".concat(config.bladeBinPath, " proposal queue --json-rpc ").concat(config.rpcUrl, " --private-key ").concat(config.validatorPk, " --input ").concat(proposal.input, " --description \"").concat(config.proposalDesc, "\"");
                    return [4 /*yield*/, executeCommand(queueCommand).then(function () {
                            console.log("Proposal queued");
                        })];
                case 2:
                    _d.sent();
                    loop = true;
                    _d.label = 3;
                case 3:
                    if (!loop) return [3 /*break*/, 6];
                    return [4 /*yield*/, sleep(10000)];
                case 4:
                    _d.sent(); // wait for 10 seconds before checking again
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x000000000000000000000000000000000000100C \"function state(uint256 proposal_id)\" ").concat(proposal.proposal_id)).then(function (output) {
                            console.log("Proposal state:", output);
                            if (output.includes('5')) {
                                console.log("Proposal is queued, proceeding to execute...");
                                loop = false;
                            }
                            else {
                                console.log("Proposal is not queued yet, waiting...");
                            }
                        })];
                case 5:
                    _d.sent();
                    return [3 /*break*/, 3];
                case 6:
                    // execute proposal
                    console.log("Executing...");
                    execCommand = "".concat(config.bladeBinPath, " proposal execute --json-rpc ").concat(config.rpcUrl, " --private-key ").concat(config.validatorPk, " --input ").concat(proposal.input, " --description \"").concat(config.proposalDesc, "\"");
                    return [4 /*yield*/, executeCommand(execCommand).then(function () {
                            console.log("Proposal executed");
                        })];
                case 7:
                    _d.sent();
                    // wait until proposal is executed
                    loop = true;
                    _d.label = 8;
                case 8:
                    if (!loop) return [3 /*break*/, 11];
                    return [4 /*yield*/, sleep(10000)];
                case 9:
                    _d.sent(); // wait for 10 seconds before checking again
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x000000000000000000000000000000000000100C \"function state(uint256 proposal_id)\" ").concat(proposal.proposal_id)).then(function (output) {
                            console.log("Proposal state:", output);
                            if (output.includes('7')) {
                                console.log("Proposal is executed, VSC starting...");
                                loop = false;
                            }
                            else {
                                console.log("Proposal is not executed yet, waiting...");
                            }
                        })];
                case 10:
                    _d.sent();
                    return [3 /*break*/, 8];
                case 11:
                    // wait until VSC is started
                    loop = true;
                    _d.label = 12;
                case 12:
                    if (!loop) return [3 /*break*/, 15];
                    return [4 /*yield*/, sleep(10000)];
                case 13:
                    _d.sent(); // wait for 10 seconds before checking again
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0xaBef000000000000000000000000000000000000 \"function isNewValidatorSetPending()\"")).then(function (output) {
                            console.log("VSC state:", output);
                            if (output.includes('1')) {
                                console.log("VSC started...");
                                loop = false;
                            }
                            else {
                                console.log("VSC not started yet, waiting...");
                            }
                        })];
                case 14:
                    _d.sent();
                    return [3 /*break*/, 12];
                case 15:
                    // wait until VSC is finished
                    loop = true;
                    _d.label = 16;
                case 16:
                    if (!loop) return [3 /*break*/, 19];
                    return [4 /*yield*/, sleep(10000)];
                case 17:
                    _d.sent(); // wait for 10 seconds before checking again
                    return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0xaBef000000000000000000000000000000000000 \"function isNewValidatorSetPending()\"")).then(function (output) {
                            console.log("VSC state:", output);
                            if (!output.includes('1')) {
                                console.log("VSC finished...");
                                loop = false;
                            }
                            else {
                                console.log("VSC in progress...");
                            }
                        })];
                case 18:
                    _d.sent();
                    return [3 /*break*/, 16];
                case 19:
                    _loop_1 = function (validator) {
                        return __generator(this, function (_e) {
                            switch (_e.label) {
                                case 0: return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x0000000000000000000000000000000000010022 \"function stakeOf(address validator)\" ").concat(validator.address)).then(function (output) {
                                        console.log("Validator %s stake:", validator.address, output);
                                        var stake = BigInt(output);
                                        if (stake > BigInt(0)) {
                                            console.log("Validator active => OK");
                                        }
                                        else {
                                            console.error("Validator inactive => ERROR");
                                        }
                                    })];
                                case 1:
                                    _e.sent();
                                    return [2 /*return*/];
                            }
                        });
                    };
                    _i = 0, _a = config.validatorsAdd;
                    _d.label = 20;
                case 20:
                    if (!(_i < _a.length)) return [3 /*break*/, 23];
                    validator = _a[_i];
                    return [5 /*yield**/, _loop_1(validator)];
                case 21:
                    _d.sent();
                    _d.label = 22;
                case 22:
                    _i++;
                    return [3 /*break*/, 20];
                case 23:
                    _loop_2 = function (validator) {
                        return __generator(this, function (_f) {
                            switch (_f.label) {
                                case 0: return [4 /*yield*/, executeCommand("cast call --rpc-url ".concat(config.rpcUrl, " 0x0000000000000000000000000000000000010022 \"function stakeOf(address validator)\" ").concat(validator)).then(function (output) {
                                        console.log("Validator %s stake:", validator, output);
                                        var stake = BigInt(output);
                                        if (stake > BigInt(0)) {
                                            console.error("Validator active => ERROR");
                                        }
                                        else {
                                            console.log("Validator inactive => OK");
                                        }
                                    })];
                                case 1:
                                    _f.sent();
                                    return [2 /*return*/];
                            }
                        });
                    };
                    _b = 0, _c = config.validatorsRemove;
                    _d.label = 24;
                case 24:
                    if (!(_b < _c.length)) return [3 /*break*/, 27];
                    validator = _c[_b];
                    return [5 /*yield**/, _loop_2(validator)];
                case 25:
                    _d.sent();
                    _d.label = 26;
                case 26:
                    _b++;
                    return [3 /*break*/, 24];
                case 27:
                    console.log("Script finished.");
                    return [2 /*return*/];
            }
        });
    });
}
// Main execution
var action = process.argv.slice(2);
var configPath = './vsc_config.json';
if (action.length > 1) {
    configPath = action[1];
}
var config = loadConfig(configPath);
if (config) {
    switch (action[0]) {
        case 'init':
            init(config);
            break;
        case 'execute':
            execute(config);
            break;
        default:
            console.log("Unknown action: ".concat(action[0]));
    }
}
