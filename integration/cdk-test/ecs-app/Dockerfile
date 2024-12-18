FROM node:lts AS builder

WORKDIR /app

COPY index.ts package.json package-lock.json ./

RUN npm ci
RUN npx esbuild --bundle index.ts --target="node18" --platform="node" --outfile="index.js"

FROM node:18-alpine
COPY --from=builder /app/index.js /app/index.js
EXPOSE 8080
CMD ["node", "/app/index.js"]
