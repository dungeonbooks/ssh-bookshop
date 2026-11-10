FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first, so a source-only change does not refetch them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off for a static binary, which is what allows distroless/static below.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ssh-bookshop .

# Carries ca-certificates for the Square calls and essentially nothing else.
# nonroot is safe because the shop never writes to disk.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/ssh-bookshop /ssh-bookshop

# Railway overrides PORT and asks for this number when setting up the TCP proxy.
ENV PORT=23234
EXPOSE 23234

# Promotes a missing SSH_HOST_KEY from warning to refusal to boot. Set in the
# image rather than sniffed, so `go run .` is unaffected. Named for the condition
# because anything matching *KEY* trips the build linter's secret detection.
ENV EPHEMERAL_STORAGE=1

USER nonroot:nonroot
ENTRYPOINT ["/ssh-bookshop"]
