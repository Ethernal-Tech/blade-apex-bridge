#!/bin/bash

BRANCH=feat/new_validator_set
BLADE_BRANCH=new-governance-change-validator-set

# build Apex-bridge smartcontracts
cd ./apex-bridge-smartcontracts
git checkout main
git fetch origin
git pull origin
if [ "$BRANCH" != "main" ]; then
    echo "SWITCHING TO ${BRANCH}"
    git branch -D ${BRANCH}
    git switch ${BRANCH}
    git pull origin # this is not important but lets have it here
fi
npm i && npx hardhat compile
cd ..

# build Apex-bridge smartcontracts
cd ./blade-contracts
git checkout main
git fetch origin
git pull origin main
if [ "$BLADE_BRANCH" != "main" ]; then
    echo "SWITCHING TO ${BLADE_BRANCH}"
    git branch -D ${BLADE_BRANCH}
    git switch ${BLADE_BRANCH}
    git pull origin ${BLADE_BRANCH} # this is not important but lets have it here
fi
npm install && npm run compile
cd ..

go run consensus/polybft/contractsapi/apex-artifacts-gen/main.go
go run consensus/polybft/contractsapi/bindings-gen/main.go
./scripts/buildb.sh