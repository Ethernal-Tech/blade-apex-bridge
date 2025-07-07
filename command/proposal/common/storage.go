package common

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

type envelope[T any] struct {
	Metadata metadata `json:"metadata"`
	Proposal T        `json:"proposal"`
}

type metadata struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
}

func LoadProposal[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if len(data) == 0 {
		var empty T
		return &empty, nil
	}

	var enve envelope[T]
	if err := json.Unmarshal(data, &enve); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file: %w", err)
	}

	return &enve.Proposal, nil
}

func StoreProposal(proposal interface{ Name() string }, path string) error {
	metadata := metadata{
		Type:      proposal.Name(),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(envelope[any]{
		Metadata: metadata,
		Proposal: proposal,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal proposal: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func GetProposalType(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	metadata := struct {
		Metadata metadata `json:"metadata"`
	}{}

	if err := json.Unmarshal(data, &metadata); err != nil {
		log.Fatalf("failed to unmarshal file: %v", err)
	}

	return metadata.Metadata.Type, nil
}
