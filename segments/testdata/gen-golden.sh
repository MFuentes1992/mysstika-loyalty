#!/usr/bin/env bash
# Regenerates segments.golden.json from joined.json with the segments filter
# copied verbatim from mysstika-ops customers.sh (the `segments` command), with
# "now" fixed at 2026-10-18T12:00:00Z. Synthetic data only: this repo is public.
set -euo pipefail
cd "$(dirname "$0")"
jq --arg month "10" --arg cutoff30 "2026-09-18T12:00:00Z" --arg cutoff45 "2026-09-03T12:00:00Z" '
    def brief: {email, firstName, stamps, lastPunch};
    {
      total: length,
      reward_ready:    (map(select(.stamps >= 10) | brief)),
      near_reward:     (map(select(.stamps >= 8 and .stamps < 10) | brief)),
      mid_cycle:       (map(select(.stamps >= 5 and .stamps < 8) | brief)),
      birthdays_this_month: (map(select(.birthDay[5:7] == $month) | {email, firstName, birthDay})),
      new_last_30d:    (map(select(.createdAt > $cutoff30) | {email, firstName, createdAt})),
      stale_45d:       (map(select(.lastPunch != "" and .lastPunch < $cutoff45) | brief)),
      never_punched:   (map(select(.lastPunch == "") | {email, firstName, createdAt})),
      missing_welcome: (map(select(.welcomeEmail | not) | {email, firstName}))
    }' <(jq 'sort_by(.createdAt)' joined.json) > segments.golden.json
