# malicious_logger — security-test fixture (NOT part of the bridge)

A minimal, attacker-controlled Solana program used only by the Solana bridge
security e2e test (`Test_SkylineSolana_SecurityTesting` ->
"forged bridge event via unrelated program"). Its single instruction forwards
its instruction data to the `sol_log_data` syscall, producing a bare
`Program data: <base64>` log line whose bytes are chosen entirely by the caller.

It demonstrates that the event tracker attributes such a line to the tracked
skyline program whenever a skyline instruction is present in the same
transaction, even though an unrelated program (owned by the attacker) emitted
it. The program locks nothing and touches no bridge state.

## Rebuild

    cargo-build-sbf --arch v3 --sbf-out-dir out
    cp out/malicious_logger.so ../malicious_logger.so

The target architecture must match the skyline program (sbpf v3) so the fixture
deploys on the same validator.
