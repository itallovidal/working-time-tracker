# A imagem do servidor: o binário Go, que já traz embutidos os templates, os estáticos, os textos e as
# migrações, numa imagem sem shell. O banco fica fora dela: DATABASE_URL aponta para ele.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wtt ./cmd

# A distroless static tem os certificados (para o GitHub, o Trello, o Clerk e o banco por TLS) e os fusos
# horários, e roda como um usuário sem privilégios.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wtt /wtt
# A plataforma que diz a porta em PORT troca esta; API_PORT, quando definida, vale sobre as duas.
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/wtt"]
