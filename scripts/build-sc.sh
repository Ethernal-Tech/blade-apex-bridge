#!/bin/bash

BRANCH=feat/AD-789_SIMPLE_colored_coins_bridging
CARDANO_SC_BRANCH=main

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

go run consensus/polybft/contractsapi/apex-artifacts-gen/main.go
go run consensus/polybft/contractsapi/bindings-gen/main.go
./scripts/buildb.sh

# Cardano smart contracts
cd ./cardano-smart-contracts
if [ "$CARDANO_SC_BRANCH" != "main" ]; then
    echo "SWITCHING TO ${CARDANO_SC_BRANCH}"
    git branch -D ${CARDANO_SC_BRANCH}
    git switch ${CARDANO_SC_BRANCH}
    git pull origin
fi
npm i
cd ..
