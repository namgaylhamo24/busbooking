#!/usr/bin/env bash
# Resilience demo: stops route-service, then shows retry with backoff, the
# circuit breaker opening, the fallback read, and recovery.
# Run from the repo root with the stack running:
#   ./scripts/resilience-demo.sh [base_url]   (default http://localhost:8080)
BASE=${1:-http://localhost:8080}
D=$(date -d '+7 days' +%F)
JSON='Content-Type: application/json'

api()  { curl -s -w "\nHTTP %{http_code}\n" "$@"; }
idof() { grep '"id"' | cut -d'"' -f4; }
create_ticket() {
  api -X POST $BASE/tickets -H "$JSON" \
    -d "{\"route_id\":\"$RID\",\"passenger_name\":\"$1\",\"travel_date\":\"$D\",\"seats\":1}"
}

echo "##### 0. Setup: one route and one ticket"
RID=$(curl -s -X POST $BASE/routes -H "$JSON" \
  -d '{"origin":"Thimphu","destination":"Phuntsholing","departure_time":"08:30","price":350,"total_seats":30}' | idof)
TID=$(curl -s -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"$RID\",\"passenger_name\":\"Karma Dorji\",\"travel_date\":\"$D\",\"seats\":2}" | idof)
echo "route=$RID ticket=$TID"

echo; echo "##### 1. Stop route-service"
docker compose stop route-service

echo; echo "##### 2. Five create calls: 1-3 retry then fail, 4-5 are rejected instantly"
for i in 1 2 3 4 5; do
  echo "--- call $i"
  time create_ticket "Test User"
done

echo; echo "##### 3. Fallback read while route-service is down (expect 200, degraded true)"
api $BASE/tickets/$TID

echo; echo "##### 4. ticket-service log (retries, backoff, breaker transitions)"
docker compose logs --tail=40 ticket-service

echo; echo "##### 5. Restart route-service, wait for the breaker probe, create again"
docker compose start route-service
sleep 12
create_ticket "Recovered User"

echo; echo "##### 6. ticket-service log (open -> half-open -> closed)"
docker compose logs --tail=8 ticket-service
