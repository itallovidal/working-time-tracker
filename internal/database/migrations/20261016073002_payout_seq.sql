-- +goose Up
-- drop index "payout_person_id_period_start_pay_date" from table: "payouts"
DROP INDEX "payout_person_id_period_start_pay_date";
-- modify "payouts" table
ALTER TABLE "payouts" ADD COLUMN "seq" bigint NOT NULL DEFAULT 0;
-- create index "payout_person_id_period_start_pay_date_seq" to table: "payouts"
CREATE UNIQUE INDEX "payout_person_id_period_start_pay_date_seq" ON "payouts" ("person_id", "period_start", "pay_date", "seq");
