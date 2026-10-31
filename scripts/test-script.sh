#!/usr/bin/env bash
set -euo pipefail

variant="${1:-v4}"
deployment="${2:-rating-service}"
namespace="${3:-${KUBE_NAMESPACE:-library-system}}"
if [[ "$variant" != v4 ]]; then
  echo "This project's cloud test scenario supports variant v4." >&2
  exit 2
fi
: "${BASE_URL:?Set BASE_URL to the cloud Gateway Ingress URL}"
base_url="${BASE_URL%/}"
if [[ ! "$base_url" =~ ^https?:// ]]; then
  echo "BASE_URL must start with http:// or https://" >&2
  exit 2
fi
wait_timeout="${TEST_WAIT_TIMEOUT:-180}"
startup_timeout="${TEST_STARTUP_WAIT_TIMEOUT:-$wait_timeout}"
if [[ ! "$wait_timeout" =~ ^[1-9][0-9]*$ || ! "$startup_timeout" =~ ^[1-9][0-9]*$ ]]; then
  echo "TEST_WAIT_TIMEOUT and TEST_STARTUP_WAIT_TIMEOUT must be positive seconds." >&2
  exit 2
fi
for command in kubectl newman curl jq openssl; do
  command -v "$command" >/dev/null
done

project_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
library_uid="83575e12-7ce0-48ee-9931-51919ff3c9ee"
book_uid="f7cdc58f-2caf-4b15-9727-f89dcc629b27"
# A fresh user avoids the rating ceiling and does not touch Test Max's history.
test_username="CI-$(date +%s)-$(openssl rand -hex 6)"
selector="app.kubernetes.io/instance=$deployment"
context_arguments=()
if [[ -n "${KUBE_CONTEXT:-}" ]]; then
  context_arguments=(--context "$KUBE_CONTEXT")
fi
kube() {
  kubectl ${context_arguments[@]+"${context_arguments[@]}"} \
    --request-timeout=15s --namespace "$namespace" "$@"
}
api() {
  curl --fail-with-body --silent --show-error --connect-timeout 5 --max-time 20 \
    --header "X-User-Name: $test_username" "$@"
}
wait_for_ingress() {
  local deadline=$((SECONDS + startup_timeout)) attempt=0 streak=0 healthy
  local probe probe_path prefix remaining max_time metrics http_status curl_status result
  mkdir -p "$report_directory/ingress-readiness"
  echo "Waiting for 3 consecutive successful Ingress probe rounds (GET only)."
  while (( SECONDS < deadline )); do
    attempt=$((attempt + 1))
    healthy=1
    for probe in health libraries books; do
      case "$probe" in
        health) probe_path=/manage/health ;;
        libraries) probe_path='/api/v1/libraries?page=1&size=10&city=%D0%9C%D0%BE%D1%81%D0%BA%D0%B2%D0%B0' ;;
        books) probe_path="/api/v1/libraries/$library_uid/books?showAll=true&page=1&size=100" ;;
      esac
      remaining=$((deadline - SECONDS))
      if (( remaining <= 0 )); then healthy=0; break; fi
      max_time=$((remaining < 10 ? remaining : 10))
      prefix="$report_directory/ingress-readiness/$attempt-$probe"
      curl_status=0
      metrics="$(curl --silent --show-error --connect-timeout 5 --max-time "$max_time" \
        --header "X-User-Name: $test_username" --output "$prefix.body" \
        --dump-header "$prefix.headers" --write-out '%{json}' \
        "$base_url$probe_path" 2>"$prefix.stderr")" || curl_status=$?
      if ! jq -e 'type == "object"' <<< "$metrics" >/dev/null 2>&1; then metrics='{}'; fi
      http_status="$(jq -r '.http_code // 0' <<< "$metrics")"
      jq --null-input --compact-output --arg url "$base_url$probe_path" \
        --argjson attempt "$attempt" --argjson curlExit "$curl_status" --argjson metrics "$metrics" \
        '{attempt: $attempt, url: $url, curlExit: $curlExit,
          httpStatus: ($metrics.http_code // 0), remoteIp: $metrics.remote_ip,
          connectSeconds: $metrics.time_connect, totalSeconds: $metrics.time_total}' \
        >> "$report_directory/ingress-readiness.jsonl"
      result=retry
      if (( curl_status == 0 )); then
        case "$http_status" in
          200) result=ready ;;
          429|5??) ;;
          *) result=fatal ;;
        esac
      else
        case "$curl_status" in
          5|6|7|18|28|52|55|56) ;; # DNS, connection, timeout or interrupted transfer.
          *) result=fatal ;;
        esac
      fi
      if [[ "$result" != ready ]]; then
        echo "Ingress $probe: HTTP=$http_status curl=$curl_status URL=$base_url$probe_path; details=$prefix.*"
        [[ "$result" == fatal ]] && return 1
        healthy=0
        break
      fi
      if [[ "$probe" != health ]] && ! jq -e '.items | type == "array"' "$prefix.body" >/dev/null; then
        echo "Ingress $probe returned HTTP 200 with an invalid page body; details=$prefix.*" >&2
        return 1
      fi
      if [[ "$probe" == books ]]; then books="$(< "$prefix.body")"; fi
    done
    if (( healthy )); then streak=$((streak + 1)); else streak=0; fi
    echo "Ingress readiness: $streak/3 consecutive rounds."
    (( streak == 3 )) && return 0
    (( SECONDS < deadline )) && sleep 1
  done
  echo "Timed out waiting for stable Ingress; see $report_directory/ingress-readiness*." >&2
  return 1
}
wait_for_rating() {
  local deadline=$((SECONDS + wait_timeout)) status
  echo "Waiting for Rating recovery through Gateway."
  while (( SECONDS < deadline )); do
    status="$(curl --silent --show-error --connect-timeout 5 --max-time 10 \
      --header "X-User-Name: $test_username" \
      --output "$report_directory/rating-readiness.json" --write-out '%{http_code}' \
      "$base_url/api/v1/rating")" || return
    case "$status" in
      200)
        if ! jq -e '.stars | type == "number"' "$report_directory/rating-readiness.json" >/dev/null; then
          echo "Rating readiness returned HTTP 200 with an invalid rating body." >&2
          return 1
        fi
        return 0
        ;;
      503) sleep 2 ;;
      *)
        echo "Rating readiness returned unexpected HTTP $status." >&2
        return 1
        ;;
    esac
  done
  echo "Timed out waiting for Rating recovery through Gateway (last HTTP $status)." >&2
  return 1
}
wait_for_endpoints() {
  local expected="$1" deadline=$((SECONDS + wait_timeout)) count
  while (( SECONDS < deadline )); do
    count="$(kube get endpointslices.discovery.k8s.io \
      -l "kubernetes.io/service-name=$deployment" -o json |
      jq '[.items[].endpoints[]? | select(.conditions.ready != false)] | length')" || return
    if [[ "$expected" == up && "$count" -gt 0 ]] ||
       [[ "$expected" == down && "$count" -eq 0 ]]; then
      return 0
    fi
    sleep 2
  done
  echo "Timed out waiting for $deployment endpoints to be $expected." >&2
  return 1
}
scale_and_wait() {
  local replicas="$1" deadline=$((SECONDS + wait_timeout)) count
  kube scale deployment "$deployment" --replicas="$replicas" || return
  if (( replicas > 0 )); then
    kube rollout status "deployment/$deployment" --timeout="${wait_timeout}s" || return
    wait_for_endpoints up || return
  else
    while :; do
      count="$(kube get pods -l "$selector" -o json | jq '.items | length')" || return
      (( count == 0 )) && break
      if (( SECONDS >= deadline )); then
        echo "Timed out waiting for $deployment Pods to disappear." >&2
        return 1
      fi
      sleep 2
    done
    wait_for_endpoints down || return
  fi
}

original_replicas="$(kube get deployment "$deployment" -o jsonpath='{.spec.replicas}')"
if [[ ! "$original_replicas" =~ ^[1-9][0-9]*$ ]]; then
  echo "$deployment must be running before tests; current replicas=$original_replicas." >&2
  exit 1
fi
temporary_directory="$(mktemp -d)"
report_directory="${TEST_REPORT_DIR:-$(mktemp -d)}"
mkdir -p "$report_directory"
environment_file="$temporary_directory/environment.json"
collection_file="$temporary_directory/collection.json"
tests_started=0
start=$SECONDS
cleanup() {
  local status=$? cleanup_status=0 reservations uid
  trap - EXIT INT TERM
  set +e
  # Explicit error checks matter: cleanup runs with errexit disabled.
  scale_and_wait "$original_replicas" || cleanup_status=1
  if (( tests_started )); then
    # Query the API, not reservationUid: a failed assertion may prevent its export.
    reservations="$(api "$base_url/api/v1/reservations")"
    if [[ $? != 0 ]] || ! jq -e 'type == "array"' <<< "$reservations" >/dev/null; then
      echo "Could not inspect reservations for $test_username; manual cleanup may be needed." >&2
      cleanup_status=1
    else
      while IFS= read -r uid; do
        [[ -z "$uid" ]] && continue
        echo "Returning unfinished test reservation $uid for $test_username."
        api --request POST --header "Content-Type: application/json" \
          --data '{"condition":"EXCELLENT","date":"2021-10-11"}' \
          "$base_url/api/v1/reservations/$uid/return" >/dev/null || cleanup_status=1
      done < <(jq -r --arg library "$library_uid" --arg book "$book_uid" \
        '.[] | select(.status == "RENTED" and .library.libraryUid == $library and .book.bookUid == $book) | .reservationUid' \
        <<< "$reservations")
    fi
  fi
  rm -f "$environment_file" "$collection_file"
  rmdir "$temporary_directory"
  printf '\nTest user: %s\nReports: %s\nTotal runtime: %s seconds\n' \
    "$test_username" "$report_directory" "$((SECONDS - start))"
  if (( cleanup_status != 0 )); then
    echo "Recovery was incomplete. Check Rating replicas and this test user's reservations." >&2
    (( status == 0 )) && status=1
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Leave the original collection and its assertions unchanged. Override only its
# fixed username in a temporary copy, after the folder's original prerequest code.
jq --arg username "$test_username" \
  '(.item[] | select(.name == "success" or .name == "failover")
    | .event[] | select(.listen == "prerequest") | .script.exec)
    += ["pm.environment.set(\"username\", " + ($username | tojson) + ");"]' \
  "$project_directory/$variant/postman/collection.json" > "$collection_file"
jq --arg base "$base_url" \
  '.values = ([.values[] | select(.key != "baseUrl")] +
    [{key: "baseUrl", value: $base, enabled: true}])' \
  "$project_directory/$variant/postman/environment.json" > "$environment_file"
jq --null-input --arg user "$test_username" --arg base "$base_url" \
  --argjson replicas "$original_replicas" \
  '{username: $user, baseUrl: $base, originalReplicas: $replicas}' \
  > "$report_directory/context.json"

echo "Testing $base_url in namespace=$namespace, username=$test_username."
kube rollout status "deployment/$deployment" --timeout="${wait_timeout}s"
wait_for_endpoints up
books=""
wait_for_ingress
# Do not cancel someone else's reservation to make the suite pass.
if ! jq -e --arg book "$book_uid" \
  '.items[] | select(.bookUid == $book and .availableCount > 0 and .condition == "EXCELLENT")' \
  <<< "$books" >/dev/null; then
  echo "The seeded test book is missing, occupied or not EXCELLENT. Restore the fixture before running; no other user's data was changed." >&2
  exit 1
fi
run_folder() {
  newman run "$collection_file" --environment "$environment_file" \
    --export-environment "$environment_file" --folder "$1" --delay-request 100 \
    --timeout-request 15000 --timeout-script 40000 --timeout 180000 \
    --reporters cli,json --reporter-json-export "$report_directory/$1.json"
}

tests_started=1
run_folder success
for step in 1 2 3 4; do
  if (( step % 2 )); then replicas=0; else replicas="$original_replicas"; fi
  echo "=== Step $step: scale $deployment to $replicas ==="
  scale_and_wait "$replicas"
  # Pod/EndpointSlice readiness alone does not prove that Gateway can reach
  # the new Pod or that its circuit breaker has completed recovery.
  if (( replicas > 0 )); then
    wait_for_rating
  fi
  run_folder "step$step"
done
