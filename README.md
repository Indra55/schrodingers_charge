# Schrödinger’s Charge

A merchant sends a ₹499 charge. The provider commits the charge, but the merchant times out before it receives the response. A fresh merchant process can check the outcome and recover the payment.

| Program | State | Interface |
| --- | --- | --- |
| Mock provider | `provider.db` | `POST /charges`, `GET /charges/{id}` |
| Merchant CLI | `merchant.db` | `pay <id> <paise>`, `status <id>`, `reconcile` |

## Run the timeout demo

Use Go 1.22 or later. Install the `sqlite3` command to inspect the provider database. Use fresh demo database paths for each run.

Build both programs from the repository root.

```sh
mkdir -p bin
go build -o bin/provider ./cmd/provider
go build -o bin/merchant ./cmd/merchant
```

In the first terminal, start the provider with a two-second delay after each new charge commits.

```sh
./bin/provider -db demo-provider.db -response-delay 2s
```

In a second terminal, send the charge. The merchant exits with status 1 after its 500 ms HTTP timeout.

```sh
./bin/merchant -db demo-merchant.db pay demo-1 49900
```

Read the local payment status. Query the provider database to confirm that it already contains one charge.

```sh
./bin/merchant -db demo-merchant.db status demo-1
# demo-1 pending (49900 paise)

sqlite3 demo-provider.db "SELECT COUNT(*) FROM charges WHERE payment_id = 'demo-1';"
# 1
```

Run `reconcile` as a fresh CLI process. The provider lookup confirms the charge and the merchant saves `paid`.

```sh
./bin/merchant -db demo-merchant.db reconcile
# demo-1 paid (49900 paise)

./bin/merchant -db demo-merchant.db status demo-1
# demo-1 paid (49900 paise)
```

Run `reconcile` again and check the provider row count.

```sh
./bin/merchant -db demo-merchant.db reconcile
# no pending payments

sqlite3 demo-provider.db "SELECT COUNT(*) FROM charges WHERE payment_id = 'demo-1';"
# 1
```

## Recovery rules

The payment ID is the idempotency key. It identifies one logical charge across all attempts. Use 1–128 ASCII letters, digits, hyphens, or underscores.

1. `pay` saves the ID and amount as `pending` before it sends the request.
2. The merchant saves `paid` after the provider confirms the same ID and amount.
3. `reconcile` looks up each pending payment. If the provider returns HTTP 404, the merchant resends the saved ID and amount.
4. If a request fails or times out, the payment stays pending for a later reconciliation.

If two workers both see HTTP 404, both can send the same charge. The provider inserts the row with `ON CONFLICT (payment_id) DO NOTHING`. The primary key allows only one insert. The other request reads and returns the committed row. The provider returns HTTP 409 if a request reuses the ID with a different amount.

Use `./bin/provider -help` and `./bin/merchant -help` to see the flags. Put merchant flags before the command.

## Run the browser demo

Run these commands from the repository root.

```sh
mkdir -p bin
go build -o bin/ ./cmd/provider ./cmd/merchant
bash ./scripts/demo.sh
```

Open `http://localhost:10000`. The Go merchant serves the page and runs each action against a separate Go provider over localhost HTTP. The page shows current state from both SQLite databases.

- Choose **Response times out after commit**, then click **Pay ₹499**. The merchant stays pending while the provider has one charge. **Reconcile** reads the existing charge and marks the payment paid.
- Choose **Request fails before commit** and click **Pay ₹499**. The provider has zero charges. Click **Race two retries** to see both requests use the same payment ID. SQLite accepts one insert.
- Click **Reset** to start with a new payment ID.

## Deploy on Render

In the Render dashboard, choose **New → Blueprint** and connect this repository. Select the branch that contains `render.yaml` and deploy. Open the `onrender.com` URL for the `schrodingers-charge` Web Service.

The Blueprint builds both Go programs and starts them on one free Web Service. The browser reaches the merchant on Render’s `PORT`. The merchant talks to the provider on localhost. The free instance clears both SQLite files on restart, redeploy, or spin-down.
