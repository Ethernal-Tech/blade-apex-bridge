package schema

type ValidatorSetChangeProposal struct {
	Added   []Validator `json:"added"`
	Removed []string    `json:"removed"`
}

type Validator struct {
	Address string         `json:"address"`
	Chains  map[string]Key `json:"chains"`
}

type Key struct {
	Key [4]string `json:"key"`
}

func (ValidatorSetChangeProposal) Name() string {
	return "validator set change proposal"
}

type EpochSizeProposal struct {
	Size int64 `json:"size"`
}

func (EpochSizeProposal) Name() string {
	return "epoch size proposal"
}

type VotingPeriodProposal struct {
	Period int64 `json:"period"`
}

func (VotingPeriodProposal) Name() string {
	return "voting period proposal"
}
