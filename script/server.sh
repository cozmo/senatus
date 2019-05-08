#!/usr/bin/env bash
#
# Run a development server locally

make

APP_ENV="development" \
  PORT="3000" \
  MONGO_URL="mongodb://localhost:27017/senatus_dev" \
  CLIENT_ID="abc123" \
  CLIENT_SECRET="client_secret" \
  REDIRECT_URI="https://localtest.me:3000/" \
  SESSION_SECRET="session_secret" \
  ./senatus
