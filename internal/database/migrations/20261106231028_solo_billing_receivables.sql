-- +goose Up
-- modify "customers" table
ALTER TABLE "customers" ADD COLUMN "billing_mode" character varying NOT NULL DEFAULT 'hourly', ADD COLUMN "default_rate_cents" bigint NULL, ADD COLUMN "monthly_fee_cents" bigint NULL, ADD COLUMN "contract_start" character varying NULL, ADD COLUMN "contract_end" character varying NULL, ADD COLUMN "closing_day" bigint NOT NULL DEFAULT 31, ADD COLUMN "payment_days" bigint NOT NULL DEFAULT 0, ADD COLUMN "late_fee_bp" bigint NOT NULL DEFAULT 0, ADD COLUMN "late_interest_bp_month" bigint NOT NULL DEFAULT 0, ADD COLUMN "grace_days" bigint NOT NULL DEFAULT 0;
-- modify "organizations" table
ALTER TABLE "organizations" ADD COLUMN "solo" boolean NOT NULL DEFAULT false;
-- create "receivables" table
CREATE TABLE "receivables" (
  "id" uuid NOT NULL,
  "kind" character varying NOT NULL,
  "period_start" character varying NOT NULL,
  "period_end" character varying NOT NULL,
  "seconds" double precision NOT NULL,
  "calculated_cents" bigint NOT NULL,
  "amount_cents" bigint NOT NULL,
  "currency" character varying NOT NULL,
  "due_date" character varying NOT NULL,
  "invoice_number" character varying NULL,
  "note" character varying NULL,
  "confirmed_at" timestamptz NOT NULL,
  "received_on" character varying NULL,
  "received_cents" bigint NULL,
  "late_fee_cents" bigint NOT NULL DEFAULT 0,
  "interest_cents" bigint NOT NULL DEFAULT 0,
  "created_at" timestamptz NOT NULL,
  "customer_id" uuid NOT NULL,
  "organization_id" uuid NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "receivables_customers_receivables" FOREIGN KEY ("customer_id") REFERENCES "customers" ("id") ON DELETE NO ACTION,
  CONSTRAINT "receivables_organizations_receivables" FOREIGN KEY ("organization_id") REFERENCES "organizations" ("id") ON DELETE CASCADE
);
-- create index "receivable_customer_id_period_start_period_end" to table: "receivables"
CREATE UNIQUE INDEX "receivable_customer_id_period_start_period_end" ON "receivables" ("customer_id", "period_start", "period_end");
