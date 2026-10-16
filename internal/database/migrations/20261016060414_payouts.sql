-- +goose Up
-- create "payouts" table
CREATE TABLE "payouts" (
  "id" uuid NOT NULL,
  "period_start" character varying NOT NULL,
  "pay_date" character varying NOT NULL,
  "seconds" double precision NOT NULL,
  "calculated_cents" bigint NOT NULL,
  "amount_cents" bigint NOT NULL,
  "paid_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL,
  "person_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "payouts_persons_payouts" FOREIGN KEY ("person_id") REFERENCES "persons" ("id") ON DELETE CASCADE
);
-- create index "payout_person_id_period_start_pay_date" to table: "payouts"
CREATE UNIQUE INDEX "payout_person_id_period_start_pay_date" ON "payouts" ("person_id", "period_start", "pay_date");
