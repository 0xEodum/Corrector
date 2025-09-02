# Используем официальный образ Go для сборки
FROM golang:1.25-alpine AS builder

# Устанавливаем необходимые пакеты для сборки
RUN apk add --no-cache git gcc musl-dev

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go.mod и go.sum для загрузки зависимостей
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем весь исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o corrector cmd/main.go

# Финальный образ
FROM alpine:latest

# Устанавливаем необходимые runtime зависимости
RUN apk --no-cache add ca-certificates

# Создаем непривилегированного пользователя
RUN addgroup -S corrector && adduser -S corrector -G corrector

WORKDIR /app

# Копируем собранное приложение из builder stage
COPY --from=builder /app/corrector .

# Копируем словари и данные (создайте эти директории, если их нет)
COPY --chown=corrector:corrector ru.txt ./
COPY --chown=corrector:corrector internal/analyzer/morph.dawg ./internal/analyzer/

# Переключаемся на непривилегированного пользователя
USER corrector

# Открываем порт
EXPOSE 8080

# Запускаем приложение
CMD ["./corrector"]