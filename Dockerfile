# Stage 1: Build the frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build the Go backend
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
# Copy the built frontend into the expected location for //go:embed
COPY --from=frontend-builder /app/dist ./frontend/dist
RUN CGO_ENABLED=0 GOOS=linux go build -o stemsorter .

# Stage 3: Final runtime image
FROM alpine:latest
RUN apk add --no-cache ffmpeg
WORKDIR /app
COPY --from=backend-builder /app/stemsorter .
EXPOSE 8080
ENTRYPOINT ["./stemsorter"]
