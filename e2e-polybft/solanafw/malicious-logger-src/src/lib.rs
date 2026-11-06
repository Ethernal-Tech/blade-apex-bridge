//! Minimal attacker-controlled Solana program used ONLY in the bridge's own
//! security e2e test. Its single instruction forwards the instruction data to
//! the `sol_log_data` syscall, producing a `Program data: <base64>` log line
//! whose bytes are fully chosen by the caller. It demonstrates that the event
//! tracker attributes such a line to the tracked (skyline) program when a
//! skyline instruction is present in the same transaction, even though this
//! program emitted it. It locks nothing and touches no bridge state.
use solana_program::{
    account_info::AccountInfo, entrypoint, entrypoint::ProgramResult,
    log::sol_log_data, pubkey::Pubkey,
};

entrypoint!(process_instruction);

fn process_instruction(
    _program_id: &Pubkey,
    _accounts: &[AccountInfo],
    instruction_data: &[u8],
) -> ProgramResult {
    // Emit the caller-supplied bytes verbatim as a `Program data:` log line.
    sol_log_data(&[instruction_data]);
    Ok(())
}
