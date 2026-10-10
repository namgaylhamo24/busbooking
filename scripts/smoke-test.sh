#!/usr/bin/env bash
# Smoke test through the API gateway.
# Usage: ./scripts/smoke-test.sh [base_url]   (default http://localhost:8080)
BASE=${1:-http://localhost:8080}
D=$(date -d '+7 days' +%F)
JSON='Content-Type: application/json'

# api "label" curl-args...  -> prints the label, the body and the HTTP status
api() { local label=$1; shift; echo; echo "=== $label"; curl -s -w "\nHTTP %{http_code}\n" "$@"; }
idof() { grep '"id"' | cut -d'"' -f4; }

echo "##### ROUTES CRUD"
api "POST /routes (expect 201)" -X POST $BASE/routes -H "$JSON" \
  -d '{"origin":"Paro","destination":"Thimphu","departure_time":"07:45","price":250,"total_seats":30}'
R1=$(curl -s -X POST $BASE/routes -H "$JSON" \
  -d '{"origin":"Wangdue","destination":"Punakha","departure_time":"10:00","price":120,"total_seats":30}' | idof)
api "GET /routes (expect 200)" $BASE/routes
api "GET /routes/{id} (expect 200)" $BASE/routes/$R1
api "PUT /routes/{id} (expect 200)" -X PUT $BASE/routes/$R1 -H "$JSON" \
  -d '{"origin":"Wangdue","destination":"Punakha","departure_time":"10:30","price":130,"total_seats":30}'

echo; echo "##### TICKETS CRUD"
api "POST /tickets (expect 201, total_price = 2 x 130)" -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"$R1\",\"passenger_name\":\"Dechen Pelden\",\"travel_date\":\"$D\",\"seats\":2}"
T1=$(curl -s -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"$R1\",\"passenger_name\":\"Tashi Norbu\",\"travel_date\":\"$D\",\"seats\":2}" | idof)
api "GET /tickets (expect 200)" $BASE/tickets
api "GET /tickets/{id} (expect 200)" $BASE/tickets/$T1
api "PUT /tickets/{id} seats 2->5 (expect 200, total_price 650)" -X PUT $BASE/tickets/$T1 -H "$JSON" \
  -d "{\"route_id\":\"$R1\",\"passenger_name\":\"Tashi Norbu\",\"travel_date\":\"$D\",\"seats\":5}"
api "DELETE /tickets/{id} (expect 204)" -X DELETE $BASE/tickets/$T1
api "GET deleted ticket (expect 404)" $BASE/tickets/$T1

echo; echo "##### ERROR CASES"
api "malformed JSON (expect 400)" -X POST $BASE/routes -H "$JSON" -d '{bad json'
api "empty body (expect 400)" -X POST $BASE/routes
api "unknown field (expect 400)" -X POST $BASE/routes -H "$JSON" -d '{"origen":"Paro"}'
api "validation failure (expect 400)" -X POST $BASE/routes -H "$JSON" \
  -d '{"origin":"Paro","destination":"Paro","departure_time":"7:45","price":-1,"total_seats":0}'
api "id not a UUID (expect 400)" $BASE/routes/abc
api "unknown route id (expect 404)" $BASE/routes/00000000-0000-0000-0000-000000000000
api "ticket for non-existent route (expect 404)" -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"00000000-0000-0000-0000-000000000000\",\"passenger_name\":\"Test User\",\"travel_date\":\"$D\",\"seats\":1}"
api "seats exceed capacity (expect 400)" -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"$R1\",\"passenger_name\":\"Test User\",\"travel_date\":\"$D\",\"seats\":31}"
api "past date (expect 400)" -X POST $BASE/tickets -H "$JSON" \
  -d "{\"route_id\":\"$R1\",\"passenger_name\":\"Test User\",\"travel_date\":\"2020-01-01\",\"seats\":1}"
api "unknown endpoint (expect 404)" $BASE/buses

echo; echo "##### DELETE ROUTE"
api "DELETE /routes/{id} (expect 204)" -X DELETE $BASE/routes/$R1
api "GET deleted route (expect 404)" $BASE/routes/$R1
