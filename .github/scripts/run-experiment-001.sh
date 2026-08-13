#!/usr/bin/env bash
set -euo pipefail

run_dir=".ariadne/ci/experiment-001"
ariadne="${RUNNER_TEMP}/ariadne"

adb -s emulator-5554 install -r \
  fixture/android/app/build/outputs/apk/debug/app-debug.apk

"${ariadne}" android check \
  --device emulator-5554 \
  --package dev.ariadne.fixture

"${ariadne}" experiment run \
  --device emulator-5554 \
  --package dev.ariadne.fixture \
  --output "${run_dir}" \
  examples/experiment-001.json

"${ariadne}" experiment report "${run_dir}"
"${ariadne}" experiment verify "${run_dir}"
verify_json="${RUNNER_TEMP}/ariadne-verify.json"
"${ariadne}" experiment verify --json "${run_dir}" >"${verify_json}"
jq -e '
  (keys_unsorted == ["manifest_name", "differences", "unknowns"]) and
  (.manifest_name == "experiment-001-email") and
  (.differences == 1) and
  (.unknowns == 0)
' "${verify_json}"

replicated_dir=".ariadne/ci/experiment-001-replicated"
"${ariadne}" experiment replicate \
  --device emulator-5554 \
  --package dev.ariadne.fixture \
  --pairs 1 \
  --output "${replicated_dir}" \
  examples/experiment-001.json
for pair_dir in \
  "${replicated_dir}/pair-001-baseline-treatment" \
  "${replicated_dir}/pair-001-treatment-baseline"; do
  "${ariadne}" experiment report "${pair_dir}"
  "${ariadne}" experiment verify "${pair_dir}"
done
replicated_verify_json="${RUNNER_TEMP}/ariadne-replicated-verify.json"
"${ariadne}" experiment replicate verify --json \
  "${replicated_dir}" >"${replicated_verify_json}"
jq -e '
  (keys_unsorted == ["schema_version", "manifest_name", "declared_variable", "receipt_sha256", "pairs", "pairs_per_order", "baseline_treatment_pairs", "treatment_baseline_pairs", "outcome", "evidence_state", "completed_pairs", "changed_pairs", "no_change_pairs", "unknown_pairs", "pair_summaries"]) and
  (.schema_version == 1) and
  (.manifest_name == "experiment-001-email") and
  (.declared_variable == "email") and
  (.receipt_sha256 | test("^[0-9a-f]{64}$")) and
  (.pairs == 2) and
  (.pairs_per_order == 1) and
  (.baseline_treatment_pairs == 1) and
  (.treatment_baseline_pairs == 1) and
  (.outcome == "replicated-change") and
  (.evidence_state == "observed") and
  (.completed_pairs == 2) and
  (.changed_pairs == 2) and
  (.no_change_pairs == 0) and
  (.unknown_pairs == 0) and
  (.pair_summaries | length == 2) and
  (all(.pair_summaries[]; .outcome == "changed" and .evidence_state == "observed" and .differences == 1 and .unknowns == 0 and (.evidence_sha256 | test("^[0-9a-f]{64}$"))))
' "${replicated_verify_json}"
if grep -F -q \
  -e "baseline@example.invalid" \
  -e "treatment@example.invalid" \
  -e "standard" \
  -e "personalized" \
  -e "request_id" \
  "${replicated_dir}/replication.json" \
  "${replicated_verify_json}"; then
  echo "replicated receipt exposed a persona or captured value" >&2
  exit 1
fi
replication_json="${replicated_dir}/replication.json"
jq -e '
  (keys_unsorted == ["schema_version", "manifest_name", "declared_variable", "pairs_per_order", "reset_policy", "status", "completed_pairs", "pairs"]) and
  (.schema_version == 1) and
  (.reset_policy == "reset-before-each-session") and
  (.status == "complete") and
  (.completed_pairs == 2) and
  (.pairs | length == 2) and
  (.pairs[0].order == "baseline-treatment" and .pairs[0].first_session == "baseline" and .pairs[0].second_session == "treatment") and
  (.pairs[1].order == "treatment-baseline" and .pairs[1].first_session == "treatment" and .pairs[1].second_session == "baseline")
' "${replication_json}"

browser_trace_json="${RUNNER_TEMP}/ariadne-browser-trace.json"
browser_trace_summary="${RUNNER_TEMP}/ariadne-browser-trace-summary.json"
"${ariadne}" browser trace --json \
  examples/browser-audit.json \
  "${browser_trace_json}" >"${browser_trace_summary}"
jq -e '
  (keys_unsorted == ["schema_version", "redacted", "scope", "completeness", "events", "trace_sha256"]) and
  (.schema_version == 1) and
  (.redacted == true) and
  (.scope == "outbound") and
  (.completeness == "complete") and
  (.events == 2) and
  (.trace_sha256 | test("^[0-9a-f]{64}$"))
' "${browser_trace_summary}"
"${ariadne}" trace verify --json "${browser_trace_json}" > /dev/null
if grep -F -q -e "https://" -e "payload" -e "cookie_value" "${browser_trace_json}"; then
  echo "browser trace exposed source details" >&2
  exit 1
fi

redacted_export_json="${RUNNER_TEMP}/ariadne-redacted-export.json"
redacted_export_stdout="${RUNNER_TEMP}/ariadne-redacted-export.stdout"
"${ariadne}" experiment export "${run_dir}" "${redacted_export_json}" >"${redacted_export_stdout}"
grep -F -x -q "redacted export complete" "${redacted_export_stdout}"
source_evidence_sha256="$(sed -n 's/^source_evidence_sha256: //p' "${redacted_export_stdout}")"
export_sha256="$(sed -n 's/^export_sha256: //p' "${redacted_export_stdout}")"
[[ "${source_evidence_sha256}" =~ ^[0-9a-f]{64}$ ]]
[[ "${export_sha256}" =~ ^[0-9a-f]{64}$ ]]
redacted_export_verify_json="${RUNNER_TEMP}/ariadne-redacted-export-verified.json"
"${ariadne}" experiment export verify --json \
  --expect-sha256 "${export_sha256}" \
  "${redacted_export_json}" >"${redacted_export_verify_json}"
jq -e --arg source_sha256 "${source_evidence_sha256}" --arg export_sha256 "${export_sha256}" '
  (keys_unsorted == ["schema_version", "source_evidence_sha256", "export_sha256"]) and
  (.schema_version == 1) and
  (.source_evidence_sha256 == $source_sha256) and
  (.export_sha256 == $export_sha256)
' "${redacted_export_verify_json}"
redacted_export_answer_json="${RUNNER_TEMP}/ariadne-redacted-export-answer.json"
"${ariadne}" experiment export ask --json \
  "${redacted_export_json}" counterfactual-change >"${redacted_export_answer_json}"
jq -e --arg source_sha256 "${source_evidence_sha256}" --arg export_sha256 "${export_sha256}" '
  (keys_unsorted == ["question_id", "question", "answer_state", "finding_ids", "source_evidence_sha256", "export_sha256"]) and
  (.question_id == "counterfactual-change") and
  (.question == "Did changing the declared variable influence an observed output?") and
  (.answer_state == "observed") and
  (.source_evidence_sha256 == $source_sha256) and
  (.export_sha256 == $export_sha256) and
  ((.finding_ids | length) == 1)
' "${redacted_export_answer_json}"
finding_id="$(jq -r '.finding_ids[0]' "${redacted_export_answer_json}")"
redacted_export_finding_json="${RUNNER_TEMP}/ariadne-redacted-export-finding.json"
"${ariadne}" experiment export finding --json \
  "${redacted_export_json}" "${finding_id}" >"${redacted_export_finding_json}"
jq -e --arg finding_id "${finding_id}" --arg source_sha256 "${source_evidence_sha256}" --arg export_sha256 "${export_sha256}" '
  (keys_unsorted == ["question", "answer_state", "kind", "classification", "id", "field", "state", "evidence", "source_evidence_sha256", "export_sha256"]) and
  (.question == "Did changing the declared variable influence an observed output?") and
  (.answer_state == "observed") and
  (.kind == "difference") and
  (.classification == "changed") and
  (.id == $finding_id) and
  (.field == "variant") and
  (.state == "observed") and
  (.source_evidence_sha256 == $source_sha256) and
  (.export_sha256 == $export_sha256) and
  ((.evidence | length) == 4)
' "${redacted_export_finding_json}"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "emulator-5554" \
  "${redacted_export_json}"; then
  echo "redacted export exposed a raw value or device identity" >&2
  exit 1
fi
list_stdout="${RUNNER_TEMP}/ariadne-list.stdout"
"${ariadne}" experiment list ".ariadne/ci" >"${list_stdout}"
grep -F -x -q "archived bundles" "${list_stdout}"
grep -F -x -q -- "- directory: experiment-001" "${list_stdout}"
list_json="${RUNNER_TEMP}/ariadne-list.json"
"${ariadne}" experiment list --json ".ariadne/ci" >"${list_json}"
jq -e '
  (length == 1) and
  (.[0] | keys_unsorted == ["directory", "manifest_name", "differences", "unknowns"]) and
  (.[0].directory == "experiment-001") and
  (.[0].manifest_name == "experiment-001-email") and
  (.[0].differences == 1) and
  (.[0].unknowns == 0)
' "${list_json}"

catalog_stdout="${RUNNER_TEMP}/ariadne-question-catalog.stdout"
"${ariadne}" experiment questions >"${catalog_stdout}"
grep -F -x -q "question catalog" "${catalog_stdout}"
grep -F -x -q -- "- id: counterfactual-change" "${catalog_stdout}"
grep -F -x -q -- "- id: capture-complete" "${catalog_stdout}"
grep -F -x -q -- "- id: source-integrity" "${catalog_stdout}"
catalog_json="${RUNNER_TEMP}/ariadne-question-catalog.json"
"${ariadne}" experiment questions --json >"${catalog_json}"
jq -e '
  (map(keys_unsorted) == [["id", "question"], ["id", "question"], ["id", "question"]]) and
  (map(.id) == ["counterfactual-change", "capture-complete", "source-integrity"]) and
  (map(.question) == [
    "Did changing the declared variable influence an observed output?",
    "Were all required observations captured for both sessions?",
    "Do the verified findings still match their source artifacts?"
  ])
' "${catalog_json}"

archive_question_json="${RUNNER_TEMP}/ariadne-archive-question.json"
archive_question_save_summary_json="${RUNNER_TEMP}/ariadne-archive-question-save-summary.json"
"${ariadne}" experiment ask-archive save --json ".ariadne/ci" counterfactual-change "${archive_question_json}" >"${archive_question_save_summary_json}"
jq -e '
  (keys_unsorted == ["schema_version", "question_id", "checked", "reflection_sha256"]) and
  (.schema_version == 2) and
  (.question_id == "counterfactual-change") and
  (.checked == 1) and
  (.reflection_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_save_summary_json}"
jq -e '
  (keys_unsorted == ["schema_version", "question_id", "question", "summary", "results"]) and
  (.schema_version == 2) and
  (.question_id == "counterfactual-change") and
  (.summary | keys_unsorted == ["observed", "unknown", "unavailable", "checked"]) and
  (.summary == {observed: 1, unknown: 0, unavailable: 0, checked: 1}) and
  (.results | length == 1) and
  (.results[0] | keys_unsorted == ["directory", "manifest_name", "recorded_at", "provenance", "answer", "available"]) and
  (.results[0].directory == "experiment-001") and
  (.results[0].provenance.manifest_contract_sha256 | length == 64) and
  (.results[0].provenance.source_evidence_sha256 | length == 64) and
  (.results[0].provenance.ariadne_revision | length > 0) and
  (.results[0].provenance.ariadne_modified == false) and
  (.results[0].available == true) and
  (.results[0].answer.answer_state == "observed")
' "${archive_question_json}"
archive_question_verified_json="${RUNNER_TEMP}/ariadne-archive-question-verified.json"
"${ariadne}" experiment ask-archive verify --json "${archive_question_json}" >"${archive_question_verified_json}"
jq -e '
  (keys_unsorted == ["schema_version", "question_id", "checked", "reflection_sha256"]) and
  (.schema_version == 2) and
  (.question_id == "counterfactual-change") and
  (.checked == 1) and
  (.reflection_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_verified_json}"
reflection_sha256="$(jq -r '.reflection_sha256' "${archive_question_verified_json}")"
"${ariadne}" experiment ask-archive verify --json --expect-sha256 "${reflection_sha256}" "${archive_question_json}" > /dev/null
archive_question_older_json="${RUNNER_TEMP}/ariadne-archive-question-older.json"
archive_question_newer_json="${RUNNER_TEMP}/ariadne-archive-question-newer.json"
cp "${archive_question_json}" "${archive_question_older_json}"
cp "${archive_question_json}" "${archive_question_newer_json}"
archive_question_comparison_json="${RUNNER_TEMP}/ariadne-archive-question-comparison.json"
"${ariadne}" experiment ask-archive compare --json "${archive_question_older_json}" "${archive_question_newer_json}" >"${archive_question_comparison_json}"
jq -e '
     (keys_unsorted == ["schema_version", "comparison_id", "comparison_question", "question_id", "question", "result", "older_reflection_sha256", "newer_reflection_sha256", "compared", "changed", "older_only", "newer_only", "state_changes"]) and
     (.schema_version == 2) and
     (.comparison_id == "answer-state-change") and
     (.comparison_question == "Did the bounded answer state change between these saved reflection snapshots?") and
  (.question_id == "counterfactual-change") and
  (.result == "same") and
  (.older_reflection_sha256 == .newer_reflection_sha256) and
  (.compared == 1) and
  (.changed == 0) and
  (.older_only == 0) and
  (.newer_only == 0) and
  (.state_changes == [])
' "${archive_question_comparison_json}"
archive_question_current_comparison_json="${RUNNER_TEMP}/ariadne-archive-question-current-comparison.json"
"${ariadne}" experiment ask-archive compare-current --json \
  "${archive_question_older_json}" \
  ".ariadne/ci" >"${archive_question_current_comparison_json}"
jq -e '
  (keys_unsorted == ["schema_version", "comparison_id", "comparison_question", "question_id", "question", "result", "older_reflection_sha256", "newer_reflection_sha256", "compared", "changed", "older_only", "newer_only", "state_changes"]) and
  (.schema_version == 2) and
  (.comparison_id == "answer-state-change") and
  (.comparison_question == "Did the bounded answer state change between these saved reflection snapshots?") and
  (.question_id == "counterfactual-change") and
  (.result == "same") and
  (.older_reflection_sha256 == .newer_reflection_sha256) and
  (.compared == 1) and
  (.changed == 0) and
  (.older_only == 0) and
  (.newer_only == 0) and
  (.state_changes == [])
' "${archive_question_current_comparison_json}"
archive_question_transitions_json="${RUNNER_TEMP}/ariadne-archive-question-transitions.json"
archive_question_transitions_save_summary_json="${RUNNER_TEMP}/ariadne-archive-question-transitions-save-summary.json"
"${ariadne}" experiment ask-archive transitions save --json \
  "${archive_question_json}" \
  "${archive_question_older_json}" \
  "${archive_question_newer_json}" \
  "${archive_question_transitions_json}" >"${archive_question_transitions_save_summary_json}"
jq -e '
  (keys_unsorted == ["schema_version", "history_id", "history_question", "question_id", "order_basis", "snapshots", "transitions", "transition_history_sha256"]) and
  (.schema_version == 3) and
  (.history_id == "answer-state-transitions") and
  (.question_id == "counterfactual-change") and
  (.order_basis == "caller") and
  (.snapshots == 3) and
  (.transitions == 2) and
  (.transition_history_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_transitions_save_summary_json}"
jq -e '
  (keys_unsorted == ["schema_version", "history_id", "history_question", "question_id", "question", "order_basis", "snapshots", "transitions", "snapshot_summaries"]) and
  (.schema_version == 3) and
  (.history_id == "answer-state-transitions") and
  (.history_question == "At which supplied boundaries did the bounded answer state change?") and
  (.question_id == "counterfactual-change") and
  (.order_basis == "caller") and
  (.snapshots == 3) and
  (.transitions | length == 2) and
  (.snapshot_summaries | length == 3) and
  (.snapshot_summaries | all(.reflection_sha256 | test("^[0-9a-f]{64}$"))) and
  (.snapshot_summaries | all((.observed + .unknown + .unavailable) == .checked)) and
  (.snapshot_summaries[0].reflection_sha256 == .transitions[0].from_reflection_sha256) and
  (.snapshot_summaries[1].reflection_sha256 == .transitions[0].to_reflection_sha256) and
  (.snapshot_summaries[2].reflection_sha256 == .transitions[1].to_reflection_sha256) and
  (.transitions | all(.from_reflection_sha256 | test("^[0-9a-f]{64}$"))) and
  (.transitions | all(.to_reflection_sha256 | test("^[0-9a-f]{64}$"))) and
  (.transitions | all(.result == "same" and .compared == 1 and .changed == 0 and .from_only == 0 and .to_only == 0))
' "${archive_question_transitions_json}"
archive_question_transitions_verified_json="${RUNNER_TEMP}/ariadne-archive-question-transitions-verified.json"
"${ariadne}" experiment ask-archive transitions verify --json \
  "${archive_question_transitions_json}" >"${archive_question_transitions_verified_json}"
jq -e '
  (keys_unsorted == ["schema_version", "history_id", "history_question", "question_id", "order_basis", "snapshots", "transitions", "transition_history_sha256"]) and
  (.schema_version == 3) and
  (.history_id == "answer-state-transitions") and
  (.history_question == "At which supplied boundaries did the bounded answer state change?") and
  (.question_id == "counterfactual-change") and
  (.order_basis == "caller") and
  (.snapshots == 3) and
  (.transitions == 2) and
  (.transition_history_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_transitions_verified_json}"
transition_history_sha256="$(jq -r '.transition_history_sha256' "${archive_question_transitions_verified_json}")"
"${ariadne}" experiment ask-archive transitions verify --json \
  --expect-sha256 "${transition_history_sha256}" \
  "${archive_question_transitions_json}" > /dev/null
archive_question_transition_answer_json="${RUNNER_TEMP}/ariadne-archive-question-transition-answer.json"
"${ariadne}" experiment ask-archive transitions ask --json \
  "${archive_question_transitions_json}" >"${archive_question_transition_answer_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "transitions", "changed_transitions", "incomparable_transitions", "changed_entries"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-transitions") and
  (.question == "At which supplied boundaries did the bounded answer state change?") and
  (.result == "same") and
  (.transition_history_sha256 == $history_sha256) and
  (.transitions == 2) and
  (.changed_transitions == []) and
  (.incomparable_transitions == []) and
  (.changed_entries == [])
' "${archive_question_transition_answer_json}"
archive_question_transition_repeated_answer_json="${RUNNER_TEMP}/ariadne-archive-question-transition-repeated-answer.json"
"${ariadne}" experiment ask-archive transitions ask repeated --json \
  "${archive_question_transitions_json}" >"${archive_question_transition_repeated_answer_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "transitions", "repeated_entries"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-repeated-changes") and
  (.question == "Did any safe archive entry change at more than one supplied boundary?") and
  (.result == "none") and
  (.transition_history_sha256 == $history_sha256) and
  (.transitions == 2) and
  (.repeated_entries == [])
' "${archive_question_transition_repeated_answer_json}"
archive_question_transition_repeated_answer_by_id_json="${RUNNER_TEMP}/ariadne-archive-question-transition-repeated-answer-by-id.json"
"${ariadne}" experiment ask-archive transitions ask --json \
  "${archive_question_transitions_json}" \
  answer-state-repeated-changes >"${archive_question_transition_repeated_answer_by_id_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (.question_id == "answer-state-repeated-changes") and
  (.transition_history_sha256 == $history_sha256) and
  (.result == "none")
' "${archive_question_transition_repeated_answer_by_id_json}"
archive_question_transition_snapshot_answer_json="${RUNNER_TEMP}/ariadne-archive-question-transition-snapshot-answer.json"
"${ariadne}" experiment ask-archive transitions ask --json \
  "${archive_question_transitions_json}" \
  answer-state-snapshot-summaries >"${archive_question_transition_snapshot_answer_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "snapshots", "snapshot_summaries"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-snapshot-summaries") and
  (.question == "What bounded answer-state summary did each supplied reflection snapshot record?") and
  (.result == "available") and
  (.transition_history_sha256 == $history_sha256) and
  (.snapshots == 3) and
  (.snapshot_summaries | length == 3) and
  (.snapshot_summaries | all(.reflection_sha256 | test("^[0-9a-f]{64}$"))) and
  (.snapshot_summaries | all((.observed + .unknown + .unavailable) == .checked))
' "${archive_question_transition_snapshot_answer_json}"
archive_question_transition_summary_answer_json="${RUNNER_TEMP}/ariadne-archive-question-transition-summary-answer.json"
"${ariadne}" experiment ask-archive transitions ask --json \
  "${archive_question_transitions_json}" \
  answer-state-summary-changes >"${archive_question_transition_summary_answer_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "transitions", "changed_transitions"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-summary-changes") and
  (.question == "Did the bounded answer-state summary change at any supplied boundary?") and
  (.result == "same") and
  (.transition_history_sha256 == $history_sha256) and
  (.transitions == 2) and
  (.changed_transitions == [])
' "${archive_question_transition_summary_answer_json}"
archive_question_transition_round_json="${RUNNER_TEMP}/ariadne-archive-question-transition-round.json"
"${ariadne}" experiment ask-archive transitions ask all --json \
  "${archive_question_transitions_json}" >"${archive_question_transition_round_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "history_question_id", "transition_history_sha256", "questions"]) and
  (.schema_version == 2) and
  (.history_question_id == "counterfactual-change") and
  (.transition_history_sha256 == $history_sha256) and
  (.questions | length == 4) and
  (.["questions"][0] == {question_id: "answer-state-transitions", question: "At which supplied boundaries did the bounded answer state change?", result: "same"}) and
  (.["questions"][1] == {question_id: "answer-state-repeated-changes", question: "Did any safe archive entry change at more than one supplied boundary?", result: "none"}) and
  (.["questions"][2] == {question_id: "answer-state-snapshot-summaries", question: "What bounded answer-state summary did each supplied reflection snapshot record?", result: "available"}) and
  (.["questions"][3] == {question_id: "answer-state-summary-changes", question: "Did the bounded answer-state summary change at any supplied boundary?", result: "same"})
' "${archive_question_transition_round_json}"
archive_question_transition_round_path="${RUNNER_TEMP}/ariadne-archive-question-transition-round-saved.json"
archive_question_transition_round_save_summary_json="${RUNNER_TEMP}/ariadne-archive-question-transition-round-save-summary.json"
"${ariadne}" experiment ask-archive transitions ask all save --json \
  "${archive_question_transitions_json}" \
  "${archive_question_transition_round_path}" >"${archive_question_transition_round_save_summary_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "transition_history_sha256", "questions", "round_sha256"]) and
  (.schema_version == 2) and
  (.transition_history_sha256 == $history_sha256) and
  (.questions == 4) and
  (.round_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_transition_round_save_summary_json}"
archive_question_transition_round_verify_summary_json="${RUNNER_TEMP}/ariadne-archive-question-transition-round-verify-summary.json"
archive_question_transition_round_sha256="$(jq -r '.round_sha256' "${archive_question_transition_round_save_summary_json}")"
"${ariadne}" experiment ask-archive transitions ask all verify --json \
  --expect-sha256 "${archive_question_transition_round_sha256}" \
  "${archive_question_transition_round_path}" >"${archive_question_transition_round_verify_summary_json}"
jq -e --slurpfile saved "${archive_question_transition_round_save_summary_json}" '
  (keys_unsorted == ["schema_version", "transition_history_sha256", "questions", "round_sha256"]) and
  (.schema_version == 2) and
  (.transition_history_sha256 == $saved[0].transition_history_sha256) and
  (.questions == $saved[0].questions) and
  (.round_sha256 == $saved[0].round_sha256)
' "${archive_question_transition_round_verify_summary_json}"
cmp -s "${archive_question_transition_round_json}" "${archive_question_transition_round_path}"
archive_question_transition_receipt_json="${RUNNER_TEMP}/ariadne-archive-question-transition-receipt.json"
"${ariadne}" experiment ask-archive transitions ask receipt --json \
  "${archive_question_transitions_json}" \
  answer-state-summary-changes >"${archive_question_transition_receipt_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "answer"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-summary-changes") and
  (.question == "Did the bounded answer-state summary change at any supplied boundary?") and
  (.result == "same") and
  (.transition_history_sha256 == $history_sha256) and
  (.answer | keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "transitions", "changed_transitions"]) and
  (.answer.schema_version == 1) and
  (.answer.result == "same") and
  (.answer.transition_history_sha256 == $history_sha256) and
  (.answer.transitions == 2) and
  (.answer.changed_transitions == [])
' "${archive_question_transition_receipt_json}"
archive_question_transition_receipt_path="${RUNNER_TEMP}/ariadne-archive-question-transition-receipt-saved.json"
archive_question_transition_receipt_save_summary_json="${RUNNER_TEMP}/ariadne-archive-question-transition-receipt-save-summary.json"
"${ariadne}" experiment ask-archive transitions ask receipt save --json \
  "${archive_question_transitions_json}" \
  answer-state-summary-changes \
  "${archive_question_transition_receipt_path}" >"${archive_question_transition_receipt_save_summary_json}"
jq -e --arg history_sha256 "${transition_history_sha256}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "receipt_sha256"]) and
  (.schema_version == 1) and
  (.question_id == "answer-state-summary-changes") and
  (.result == "same") and
  (.transition_history_sha256 == $history_sha256) and
  (.receipt_sha256 | test("^[0-9a-f]{64}$"))
' "${archive_question_transition_receipt_save_summary_json}"
archive_question_transition_receipt_verify_summary_json="${RUNNER_TEMP}/ariadne-archive-question-transition-receipt-verify-summary.json"
archive_question_transition_receipt_sha256="$(jq -r '.receipt_sha256' "${archive_question_transition_receipt_save_summary_json}")"
"${ariadne}" experiment ask-archive transitions ask receipt verify --json \
  --expect-sha256 "${archive_question_transition_receipt_sha256}" \
  "${archive_question_transition_receipt_path}" >"${archive_question_transition_receipt_verify_summary_json}"
jq -e --slurpfile saved "${archive_question_transition_receipt_save_summary_json}" '
  (keys_unsorted == ["schema_version", "question_id", "question", "result", "transition_history_sha256", "receipt_sha256"]) and
  (.schema_version == 1) and
  (.question_id == $saved[0].question_id) and
  (.question == $saved[0].question) and
  (.result == $saved[0].result) and
  (.transition_history_sha256 == $saved[0].transition_history_sha256) and
  (.receipt_sha256 == $saved[0].receipt_sha256)
' "${archive_question_transition_receipt_verify_summary_json}"
cmp -s "${archive_question_transition_receipt_json}" "${archive_question_transition_receipt_path}"
archive_question_transition_questions_json="${RUNNER_TEMP}/ariadne-archive-question-transition-questions.json"
"${ariadne}" experiment ask-archive transitions questions --json >"${archive_question_transition_questions_json}"
jq -e '
  (length == 4) and
  (.[0] == {id: "answer-state-transitions", question: "At which supplied boundaries did the bounded answer state change?"}) and
  (.[1] == {id: "answer-state-repeated-changes", question: "Did any safe archive entry change at more than one supplied boundary?"}) and
  (.[2] == {id: "answer-state-snapshot-summaries", question: "What bounded answer-state summary did each supplied reflection snapshot record?"}) and
  (.[3] == {id: "answer-state-summary-changes", question: "Did the bounded answer-state summary change at any supplied boundary?"})
' "${archive_question_transition_questions_json}"

finding_id="$(jq -r '.comparison.differences[0].id' "${run_dir}/evidence.json")"
finding_stdout="${RUNNER_TEMP}/ariadne-finding.stdout"
"${ariadne}" experiment finding "${run_dir}" "${finding_id}" >"${finding_stdout}"
grep -F -x -q "finding verified" "${finding_stdout}"
grep -F -x -q "kind: difference" "${finding_stdout}"
grep -F -x -q "field: variant" "${finding_stdout}"
grep -F -q "baseline/observations/storage.json#/variant" "${finding_stdout}"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "request_id" \
  "${finding_stdout}"; then
  echo "finding lookup exposed observed value" >&2
  exit 1
fi
finding_json="${RUNNER_TEMP}/ariadne-finding.json"
"${ariadne}" experiment finding --json "${run_dir}" "${finding_id}" >"${finding_json}"
jq -e '
  (keys_unsorted == ["question", "answer_state", "kind", "classification", "id", "field", "state", "evidence"]) and
  (.answer_state == "observed") and
  (.kind == "difference") and
  (.classification == "changed") and
  (.id | test("^sha256:[0-9a-f]{64}$")) and
  (.field == "variant") and
  (.state == "observed") and
  (.evidence == [
    "baseline/observations/storage.json#/variant",
    "baseline/observations/network.json#decoded-body/variant",
    "treatment/observations/storage.json#/variant",
    "treatment/observations/network.json#decoded-body/variant"
  ])
' "${finding_json}"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "request_id" \
  "${finding_json}"; then
  echo "JSON finding lookup exposed observed value" >&2
  exit 1
fi
for question_id in counterfactual-change capture-complete source-integrity; do
  question_stdout="${RUNNER_TEMP}/ariadne-question-${question_id}.stdout"
  "${ariadne}" experiment ask "${run_dir}" "${question_id}" >"${question_stdout}"
  grep -F -x -q "question answered" "${question_stdout}"
  grep -F -x -q "id: ${question_id}" "${question_stdout}"
  grep -F -q "sha256:" "${question_stdout}"
  if grep -F -q \
    -e "standard" \
    -e "personalized" \
    -e "request_id" \
    "${question_stdout}"; then
    echo "question answer exposed observed value" >&2
    exit 1
  fi
  question_json="${RUNNER_TEMP}/ariadne-question-${question_id}.json"
  "${ariadne}" experiment ask --json "${run_dir}" "${question_id}" >"${question_json}"
  jq -e \
    --arg question_id "${question_id}" \
    '
    (keys_unsorted == ["question_id", "question", "answer_state", "finding_ids"]) and
    (.question_id == $question_id) and
    (.question | type == "string") and
    (.answer_state == "observed") and
    (.finding_ids | length == 1) and
    all(.finding_ids[]; test("^sha256:[0-9a-f]{64}$"))
    ' "${question_json}"
  if grep -F -q \
    -e "standard" \
    -e "personalized" \
    -e "request_id" \
    "${question_json}"; then
    echo "JSON question answer exposed observed value" >&2
    exit 1
  fi
done

fixture_sha256="$(
  sha256sum fixture/android/app/build/outputs/apk/debug/app-debug.apk |
    cut -d " " -f 1
)"
jq -e \
  --arg fixture_sha256 "${fixture_sha256}" \
  --arg ariadne_revision "${GITHUB_SHA}" '
  (.schema_version == 7) and
  (.manifest_contract_sha256 | test("^[0-9a-f]{64}$")) and
  (.question == "Did changing email influence an observed output?") and
  (.answer_state == "observed") and
  (.target.android_api == 35) and
  (.target.architecture == "x86_64") and
  (.target.package_version_code == 1) and
  (.target.package_sha256 == $fixture_sha256) and
  (.target.ariadne_revision == $ariadne_revision) and
  (.target.ariadne_modified == false) and
  (.artifacts | length == 6) and
  (.comparison.schema_version == 5) and
  (.comparison.unchanged_fields == ["region"]) and
  (.comparison.normalized_fields == ["request_id"]) and
  (.comparison.differences | length == 1) and
  (.comparison.unknowns == []) and
  (
    .normalizations |
    index("removed declared volatile observation field request_id from comparison") != null
  ) and
  (
    .comparison.differences[0] |
    .field == "variant" and
    .kind == "changed" and
    (.id | test("^sha256:[0-9a-f]{64}$")) and
    .baseline == "standard" and
    .treatment == "personalized" and
    .state == "observed"
  )
' "${run_dir}/evidence.json"

contract_digest="$(jq -r '.manifest_contract_sha256' "${run_dir}/evidence.json")"

tap_resource_id="dev.ariadne.fixture:id/observe_button"
for session in baseline treatment; do
  jq -e \
    --arg tap_resource_id "${tap_resource_id}" \
    --arg contract_digest "${contract_digest}" \
    '
    (.schema_version == 7) and
    (.tap_resource_id == $tap_resource_id) and
    (.manifest_contract_sha256 == $contract_digest) and
    (.status == "complete") and
    any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0 and (.ui_hierarchy_sha256 | test("^[0-9a-f]{64}$")))
    ' "${run_dir}/${session}/session.json"
done

baseline_request_id="$(
  jq -r '.request_id' "${run_dir}/baseline/observations/storage.json"
)"
treatment_request_id="$(
  jq -r '.request_id' "${run_dir}/treatment/observations/storage.json"
)"
test -n "${baseline_request_id}"
test -n "${treatment_request_id}"
test "${baseline_request_id}" != "${treatment_request_id}"
if grep -F -q \
  -e "${baseline_request_id}" \
  -e "${treatment_request_id}" \
  "${run_dir}/evidence.json" \
  "${run_dir}/report.md"; then
  echo "volatile value found in normalized evidence output" >&2
  exit 1
fi
grep -F -q \
  "removed declared volatile observation field request_id from comparison" \
  "${run_dir}/report.md"

baseline_email="$(jq -r '.baseline.email' examples/experiment-001.json)"
treatment_email="$(jq -r '.treatment.email' examples/experiment-001.json)"
failure_dir="${RUNNER_TEMP}/ariadne-failure-proofs"
mkdir "${failure_dir}"

if grep -R -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  "${run_dir}"; then
  echo "persona value found in evidence output" >&2
  exit 1
fi

complete_trace_dir="${failure_dir}/complete-traces"
mkdir "${complete_trace_dir}"
complete_baseline_trace="${complete_trace_dir}/baseline.json"
complete_treatment_trace="${complete_trace_dir}/treatment.json"
complete_baseline_trace_stdout="${complete_trace_dir}/baseline.stdout"
complete_treatment_trace_stdout="${complete_trace_dir}/treatment.stdout"
"${ariadne}" experiment trace --session baseline "${run_dir}" "${complete_baseline_trace}" >"${complete_baseline_trace_stdout}"
"${ariadne}" experiment trace --session treatment "${run_dir}" "${complete_treatment_trace}" >"${complete_treatment_trace_stdout}"
grep -F -x -q "completeness: complete" "${complete_baseline_trace_stdout}"
grep -F -x -q "completeness: complete" "${complete_treatment_trace_stdout}"
complete_baseline_trace_sha256="$(sed -n 's/^trace_sha256: //p' "${complete_baseline_trace_stdout}")"
complete_treatment_trace_sha256="$(sed -n 's/^trace_sha256: //p' "${complete_treatment_trace_stdout}")"
[[ "${complete_baseline_trace_sha256}" =~ ^[0-9a-f]{64}$ ]]
[[ "${complete_treatment_trace_sha256}" =~ ^[0-9a-f]{64}$ ]]
"${ariadne}" trace verify --json --expect-sha256 "${complete_baseline_trace_sha256}" "${complete_baseline_trace}" >"${complete_trace_dir}/baseline-verify.json"
"${ariadne}" trace verify --json --expect-sha256 "${complete_treatment_trace_sha256}" "${complete_treatment_trace}" >"${complete_trace_dir}/treatment-verify.json"
jq -e '
  (.scope == "all") and
  (.completeness == "complete") and
  (.events == 2) and
  (.trace_sha256 | test("^[0-9a-f]{64}$"))
' "${complete_trace_dir}/baseline-verify.json"
complete_trace_comparison="${complete_trace_dir}/comparison.json"
"${ariadne}" trace compare --json "${complete_baseline_trace}" "${complete_treatment_trace}" >"${complete_trace_comparison}"
jq -e '
  (.scope == "all") and
  (.baseline_completeness == "complete") and
  (.treatment_completeness == "complete") and
  (.unchanged | length == 2) and
  (.differences | length == 0) and
  (.unknowns | length == 0)
' "${complete_trace_comparison}"
complete_baseline_session="${complete_trace_dir}/baseline-session.json"
complete_treatment_session="${complete_trace_dir}/treatment-session.json"
complete_pair_summary="${complete_trace_dir}/pair-summary.json"
"${ariadne}" trace session pair create --json \
  --adapter android-experiment-001 \
  --adapter-version 1 \
  --procedure-sha256 "${contract_digest}" \
  --order baseline-treatment \
  "${complete_baseline_trace}" "${complete_treatment_trace}" \
  "${complete_baseline_session}" "${complete_treatment_session}" >"${complete_pair_summary}"
jq -e \
  --arg contract_digest "${contract_digest}" \
  '
  (keys_unsorted == ["schema_version", "pair_sha256", "source", "adapter", "adapter_version", "procedure_sha256", "scope", "order", "baseline_trace_sha256", "treatment_trace_sha256", "baseline_completeness", "treatment_completeness", "baseline_session_sha256", "treatment_session_sha256"]) and
  (.schema_version == 1) and
  (.source == "android") and
  (.adapter == "android-experiment-001") and
  (.adapter_version == 1) and
  (.procedure_sha256 == $contract_digest) and
  (.scope == "all") and
  (.order == "baseline-treatment") and
  (.baseline_trace_sha256 | test("^[0-9a-f]{64}$")) and
  (.treatment_trace_sha256 | test("^[0-9a-f]{64}$")) and
  (.baseline_completeness == "complete") and
  (.treatment_completeness == "complete") and
  (.baseline_session_sha256 | test("^[0-9a-f]{64}$")) and
  (.treatment_session_sha256 | test("^[0-9a-f]{64}$")) and
  (.pair_sha256 | test("^[0-9a-f]{64}$"))
  ' "${complete_pair_summary}"
complete_baseline_session_sha256="$(jq -r '.baseline_session_sha256' "${complete_pair_summary}")"
complete_treatment_session_sha256="$(jq -r '.treatment_session_sha256' "${complete_pair_summary}")"
"${ariadne}" trace session verify --json \
  --expect-sha256 "${complete_baseline_session_sha256}" \
  "${complete_baseline_session}" "${complete_baseline_trace}" >"${complete_trace_dir}/baseline-session-verify.json"
"${ariadne}" trace session verify --json \
  --expect-sha256 "${complete_treatment_session_sha256}" \
  "${complete_treatment_session}" "${complete_treatment_trace}" >"${complete_trace_dir}/treatment-session-verify.json"
"${ariadne}" trace session pair verify --json \
  "${complete_baseline_session}" "${complete_baseline_trace}" \
  "${complete_treatment_session}" "${complete_treatment_trace}" >"${complete_pair_summary}"
jq -e \
  --arg contract_digest "${contract_digest}" \
  --arg baseline_trace_sha256 "${complete_baseline_trace_sha256}" \
  --arg treatment_trace_sha256 "${complete_treatment_trace_sha256}" \
  --arg baseline_session_sha256 "${complete_baseline_session_sha256}" \
  --arg treatment_session_sha256 "${complete_treatment_session_sha256}" \
  '
  (keys_unsorted == ["schema_version", "pair_sha256", "source", "adapter", "adapter_version", "procedure_sha256", "scope", "order", "baseline_trace_sha256", "treatment_trace_sha256", "baseline_completeness", "treatment_completeness", "baseline_session_sha256", "treatment_session_sha256"]) and
  (.schema_version == 1) and
  (.source == "android") and
  (.adapter == "android-experiment-001") and
  (.adapter_version == 1) and
  (.procedure_sha256 == $contract_digest) and
  (.scope == "all") and
  (.order == "baseline-treatment") and
  (.baseline_trace_sha256 == $baseline_trace_sha256) and
  (.treatment_trace_sha256 == $treatment_trace_sha256) and
  (.baseline_completeness == "complete") and
  (.treatment_completeness == "complete") and
  (.baseline_session_sha256 == $baseline_session_sha256) and
  (.treatment_session_sha256 == $treatment_session_sha256)
  ' "${complete_pair_summary}"
if grep -R -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  -e "${baseline_request_id}" \
  -e "${treatment_request_id}" \
  -e "standard" \
  -e "personalized" \
  "${complete_trace_dir}"; then
  echo "complete trace exposed a captured value" >&2
  exit 1
fi

storage_gap_dir=".ariadne/ci/experiment-001-storage-gap"
storage_gap_stdout="${failure_dir}/storage-gap-run.stdout"
storage_gap_stderr="${failure_dir}/storage-gap-run.stderr"
if "${ariadne}" experiment run \
  --device emulator-5554 \
  --package dev.ariadne.fixture \
  --output "${storage_gap_dir}" \
  examples/experiment-001-storage-gap.json \
  >"${storage_gap_stdout}" 2>"${storage_gap_stderr}"; then
  echo "storage gap: experiment run unexpectedly succeeded" >&2
  exit 1
fi
if [[ -s "${storage_gap_stdout}" ]]; then
  echo "storage gap: failed run wrote to stdout" >&2
  exit 1
fi
if ! grep -F -q "capture storage" "${storage_gap_stderr}"; then
  echo "storage gap: expected capture failure was not reported" >&2
  cat "${storage_gap_stderr}" >&2
  exit 1
fi
if grep -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  "${storage_gap_stderr}"; then
  echo "storage gap: failure output disclosed a persona value" >&2
  exit 1
fi

test -e "${storage_gap_dir}/baseline/observations/storage.json"
test -e "${storage_gap_dir}/baseline/observations/network.json"
test -e "${storage_gap_dir}/treatment/observations/network.json"
test ! -e "${storage_gap_dir}/treatment/observations/storage.json"
storage_gap_contract_digest="$(jq -r '.manifest_contract_sha256' "${storage_gap_dir}/baseline/session.json")"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.status == "complete") and
  (.schema_version == 7) and
  (.tap_resource_id == "dev.ariadne.fixture:id/observe_button") and
  (.manifest_contract_sha256 == $contract_digest) and
  any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0 and (.ui_hierarchy_sha256 | test("^[0-9a-f]{64}$"))) and
  (.artifacts | length == 2)
' "${storage_gap_dir}/baseline/session.json"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.status == "incomplete") and
  (.schema_version == 7) and
  (.tap_resource_id == "dev.ariadne.fixture:id/observe_button") and
  (.manifest_contract_sha256 == $contract_digest) and
  any(.steps[]; .name == "interact" and .status == "ok" and .exit_code == 0 and (.ui_hierarchy_sha256 | test("^[0-9a-f]{64}$"))) and
  (.failure_stage == "capture_storage") and
  (.artifacts | length == 1) and
  (.artifacts[0].path == "observations/network.json") and
  any(.steps[]; .name == "capture_storage" and .status == "error")
' "${storage_gap_dir}/treatment/session.json"

storage_gap_baseline_request_id="$(
  jq -r '.request_id' \
    "${storage_gap_dir}/baseline/observations/storage.json"
)"
storage_gap_treatment_request_id="$(
  jq -r '.body_base64 | @base64d | fromjson | .request_id' \
    "${storage_gap_dir}/treatment/observations/network.json"
)"
test -n "${storage_gap_baseline_request_id}"
test -n "${storage_gap_treatment_request_id}"
test "${storage_gap_baseline_request_id}" != "${storage_gap_treatment_request_id}"

storage_gap_report_stdout="${failure_dir}/storage-gap-report.stdout"
"${ariadne}" experiment report "${storage_gap_dir}" >"${storage_gap_report_stdout}"
"${ariadne}" experiment verify "${storage_gap_dir}" >"${failure_dir}/storage-gap-verify.stdout"
storage_gap_verify_json="${failure_dir}/storage-gap-verify.json"
"${ariadne}" experiment verify --json "${storage_gap_dir}" >"${storage_gap_verify_json}"
jq -e '
  (keys_unsorted == ["manifest_name", "differences", "unknowns"]) and
  (.manifest_name == "experiment-001-email-storage-gap") and
  (.differences == 0) and
  (.unknowns == 3)
' "${storage_gap_verify_json}"
storage_gap_trace_dir="${failure_dir}/storage-gap-traces"
mkdir "${storage_gap_trace_dir}"
storage_gap_baseline_trace="${storage_gap_trace_dir}/baseline.json"
storage_gap_treatment_trace="${storage_gap_trace_dir}/treatment.json"
storage_gap_baseline_trace_stdout="${storage_gap_trace_dir}/baseline.stdout"
storage_gap_treatment_trace_stdout="${storage_gap_trace_dir}/treatment.stdout"
"${ariadne}" experiment trace --session baseline "${storage_gap_dir}" "${storage_gap_baseline_trace}" >"${storage_gap_baseline_trace_stdout}"
"${ariadne}" experiment trace --session treatment "${storage_gap_dir}" "${storage_gap_treatment_trace}" >"${storage_gap_treatment_trace_stdout}"
grep -F -x -q "completeness: complete" "${storage_gap_baseline_trace_stdout}"
grep -F -x -q "completeness: partial" "${storage_gap_treatment_trace_stdout}"
storage_gap_trace_comparison="${storage_gap_trace_dir}/comparison.json"
"${ariadne}" trace compare --json "${storage_gap_baseline_trace}" "${storage_gap_treatment_trace}" >"${storage_gap_trace_comparison}"
jq -e '
  (.scope == "all") and
  (.baseline_completeness == "complete") and
  (.treatment_completeness == "partial") and
  (.unchanged | length == 1) and
  (.differences | length == 0) and
  (.unknowns | length == 1) and
  (.unknowns[0].state == "unknown")
' "${storage_gap_trace_comparison}"
if grep -R -F -q \
  -e "${baseline_email}" \
  -e "${treatment_email}" \
  -e "${storage_gap_baseline_request_id}" \
  -e "${storage_gap_treatment_request_id}" \
  -e "standard" \
  -e "personalized" \
  "${storage_gap_trace_dir}"; then
  echo "storage gap trace exposed a captured value" >&2
  exit 1
fi
storage_gap_finding_id="$(jq -r '.comparison.unknowns[0].id' "${storage_gap_dir}/evidence.json")"
"${ariadne}" experiment finding "${storage_gap_dir}" "${storage_gap_finding_id}" >"${failure_dir}/storage-gap-finding.stdout"
grep -F -x -q "finding verified" "${failure_dir}/storage-gap-finding.stdout"
grep -F -x -q "answer_state: unknown" "${failure_dir}/storage-gap-finding.stdout"
grep -F -x -q "kind: unknown" "${failure_dir}/storage-gap-finding.stdout"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "request_id" \
  "${failure_dir}/storage-gap-finding.stdout"; then
  echo "storage gap finding lookup exposed observed value" >&2
  exit 1
fi
storage_gap_finding_json="${failure_dir}/storage-gap-finding.json"
"${ariadne}" experiment finding --json "${storage_gap_dir}" "${storage_gap_finding_id}" >"${storage_gap_finding_json}"
jq -e '
  (keys_unsorted == ["question", "answer_state", "kind", "id", "field", "state", "reason", "evidence"]) and
  (.answer_state == "unknown") and
  (.kind == "unknown") and
  (.id | test("^sha256:[0-9a-f]{64}$")) and
  (.state == "unknown") and
  (.reason == "treatment storage observation was not captured") and
  (.evidence | length == 3)
' "${storage_gap_finding_json}"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "request_id" \
  "${storage_gap_finding_json}"; then
  echo "storage gap JSON finding lookup exposed observed value" >&2
  exit 1
fi
for question_id in counterfactual-change capture-complete source-integrity; do
  question_stdout="${failure_dir}/storage-gap-question-${question_id}.stdout"
  "${ariadne}" experiment ask "${storage_gap_dir}" "${question_id}" >"${question_stdout}"
  grep -F -x -q "question answered" "${question_stdout}"
  grep -F -x -q "id: ${question_id}" "${question_stdout}"
  if [[ "${question_id}" == "source-integrity" ]]; then
    grep -F -x -q "answer_state: observed" "${question_stdout}"
  else
    grep -F -x -q "answer_state: unknown" "${question_stdout}"
  fi
  if grep -F -q \
    -e "standard" \
    -e "personalized" \
    -e "request_id" \
    "${question_stdout}"; then
    echo "storage gap question answer exposed observed value" >&2
    exit 1
  fi
  question_json="${failure_dir}/storage-gap-question-${question_id}.json"
  expected_state="unknown"
  expected_reason="treatment storage observation was not captured"
  if [[ "${question_id}" == "source-integrity" ]]; then
    expected_state="observed"
    expected_reason=""
  fi
  "${ariadne}" experiment ask --json "${storage_gap_dir}" "${question_id}" >"${question_json}"
  jq -e \
    --arg question_id "${question_id}" \
    --arg expected_state "${expected_state}" \
    --arg expected_reason "${expected_reason}" \
    '
    (.question_id == $question_id) and
    (.question | type == "string") and
    (.answer_state == $expected_state) and
    (if $expected_reason == "" then
       (keys_unsorted == ["question_id", "question", "answer_state", "finding_ids"]) and (has("reason") | not)
     else
       (keys_unsorted == ["question_id", "question", "answer_state", "reason", "finding_ids"]) and (.reason == $expected_reason)
     end) and
    (.finding_ids | length == 3) and
    all(.finding_ids[]; test("^sha256:[0-9a-f]{64}$"))
    ' "${question_json}"
  if grep -F -q \
    -e "standard" \
    -e "personalized" \
    -e "request_id" \
    "${question_json}"; then
    echo "storage gap JSON question answer exposed observed value" >&2
    exit 1
  fi
done
grep -F -x -q "differences: 0" "${storage_gap_report_stdout}"
grep -F -x -q "unknowns: 3" "${storage_gap_report_stdout}"
jq -e \
  --arg contract_digest "${storage_gap_contract_digest}" \
  '
  (.schema_version == 7) and
  (.manifest_contract_sha256 == $contract_digest) and
  (.question == "Did changing email influence an observed output?") and
  (.answer_state == "unknown") and
  (.manifest_name == "experiment-001-email-storage-gap") and
  (.artifacts | length == 5) and
  (.comparison.schema_version == 5) and
  (.comparison.unchanged_fields == []) and
  (.comparison.normalized_fields == []) and
  (.comparison.differences == []) and
  (.comparison.unknowns | map(.field) == ["region", "request_id", "variant"]) and
  all(
    .comparison.unknowns[];
    (.id | test("^sha256:[0-9a-f]{64}$")) and
    .state == "unknown" and
    .reason == "treatment storage observation was not captured" and
    (.evidence | length == 3)
  )
' "${storage_gap_dir}/evidence.json"
if grep -F -q \
  -e "standard" \
  -e "personalized" \
  -e "${storage_gap_baseline_request_id}" \
  -e "${storage_gap_treatment_request_id}" \
  "${storage_gap_dir}/evidence.json" \
  "${storage_gap_dir}/report.md"; then
  echo "storage gap: partial report disclosed an observed value" >&2
  exit 1
fi

expect_failure() {
  local name="$1"
  local expected_error="$2"
  shift 2

  local stdout="${failure_dir}/${name}.stdout"
  local stderr="${failure_dir}/${name}.stderr"
  if "$@" >"${stdout}" 2>"${stderr}"; then
    echo "${name}: command unexpectedly succeeded" >&2
    exit 1
  fi
  if [[ -s "${stdout}" ]]; then
    echo "${name}: failed command wrote to stdout" >&2
    exit 1
  fi
  if ! grep -F -q "${expected_error}" "${stderr}"; then
    echo "${name}: expected error was not reported" >&2
    exit 1
  fi
  if grep -F -q \
    -e "${baseline_email}" \
    -e "${treatment_email}" \
    -e "private-tamper-marker" \
    "${stderr}"; then
    echo "${name}: failure output disclosed protected input" >&2
    exit 1
  fi
}

expect_failure \
  "missing-package" \
  "ariadne: android check:" \
  "${ariadne}" android check \
  --device emulator-5554 \
  --package dev.ariadne.missing

artifact_dir="${failure_dir}/artifact"
cp -R "${run_dir}" "${artifact_dir}"
printf '\nprivate-tamper-marker\n' >> \
  "${artifact_dir}/baseline/observations/storage.json"
expect_failure \
  "artifact-verify" \
  "integrity check failed" \
  "${ariadne}" experiment verify "${artifact_dir}"
expect_failure \
  "artifact-verify-json" \
  "integrity check failed" \
  "${ariadne}" experiment verify --json "${artifact_dir}"
expect_failure \
  "artifact-finding" \
  "integrity check failed" \
  "${ariadne}" experiment finding "${artifact_dir}" "${finding_id}"
expect_failure \
  "artifact-finding-json" \
  "integrity check failed" \
  "${ariadne}" experiment finding --json "${artifact_dir}" "${finding_id}"
expect_failure \
  "artifact-ask" \
  "integrity check failed" \
  "${ariadne}" experiment ask "${artifact_dir}" "counterfactual-change"
expect_failure \
  "artifact-ask-json" \
  "integrity check failed" \
  "${ariadne}" experiment ask --json "${artifact_dir}" "counterfactual-change"
test -e "${artifact_dir}/evidence.json"
test -e "${artifact_dir}/report.md"
rm "${artifact_dir}/evidence.json" "${artifact_dir}/report.md"
expect_failure \
  "artifact-integrity" \
  "integrity check failed" \
  "${ariadne}" experiment report "${artifact_dir}"
test ! -e "${artifact_dir}/evidence.json"
test ! -e "${artifact_dir}/report.md"

intermediate_list_root="${failure_dir}/intermediate-list"
mkdir "${intermediate_list_root}"
intermediate_symlink_dir="${intermediate_list_root}/intermediate-symlink"
cp -R "${run_dir}" "${intermediate_symlink_dir}"
outside_observations="${failure_dir}/outside-observations"
mv "${intermediate_symlink_dir}/baseline/observations" "${outside_observations}"
ln -s "${outside_observations}" "${intermediate_symlink_dir}/baseline/observations"
expect_failure \
  "intermediate-symlink-verify" \
  "symbolic links are not allowed" \
  "${ariadne}" experiment verify --json "${intermediate_symlink_dir}"
expect_failure \
  "intermediate-symlink-list" \
  "symbolic links are not allowed" \
  "${ariadne}" experiment list --json "${intermediate_list_root}"

expect_failure \
  "unknown-finding" \
  "finding not found" \
  "${ariadne}" experiment finding "${run_dir}" \
  "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
expect_failure \
  "invalid-finding-json-flag" \
  "usage:" \
  "${ariadne}" experiment finding --json=invalid "${run_dir}" "${finding_id}"
expect_failure \
  "invalid-verify-json-flag" \
  "usage:" \
  "${ariadne}" experiment verify --json=invalid "${run_dir}"
expect_failure \
  "invalid-list-json-flag" \
  "usage:" \
  "${ariadne}" experiment list --json=invalid ".ariadne/ci"
expect_failure \
  "unknown-question" \
  "question ID is invalid" \
  "${ariadne}" experiment ask "${run_dir}" "not-a-question"
expect_failure \
  "invalid-json-flag" \
  "usage:" \
  "${ariadne}" experiment ask --json=invalid "${run_dir}" "counterfactual-change"

provenance_dir="${failure_dir}/provenance"
cp -R "${run_dir}" "${provenance_dir}"
rm "${provenance_dir}/evidence.json" "${provenance_dir}/report.md"
jq '.package_sha256 = "0000000000000000000000000000000000000000000000000000000000000000"' \
  "${provenance_dir}/treatment/session.json" > \
  "${provenance_dir}/treatment/session.json.tmp"
mv \
  "${provenance_dir}/treatment/session.json.tmp" \
  "${provenance_dir}/treatment/session.json"
expect_failure \
  "provenance-mismatch" \
  "session metadata disagree" \
  "${ariadne}" experiment report "${provenance_dir}"
test ! -e "${provenance_dir}/evidence.json"
test ! -e "${provenance_dir}/report.md"
