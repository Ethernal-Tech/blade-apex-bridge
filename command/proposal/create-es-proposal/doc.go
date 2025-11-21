package createesproposal

import "github.com/0xPolygon/polygon-edge/command/proposal/common"

var doc string = `This command is used to create an epoch size governance proposal (and potentially submit it to the
governance system). Only one flag is required for the command to function correctly (--epoch-size).
However, note that if you don't provide neither --file nor --submit, the command becomes a no-op.

The required flag --epoch-size represents the new (proposed) size of the epoch. The provided value
must be an integer greater than zero.

The optional flag --file specifies the path to the proposal file that will be created. If the file
already exists, its contents will be truncated.` + common.SubmitRelatedFlagsDoc
