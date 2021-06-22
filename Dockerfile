FROM alpine
EXPOSE 8080
EXPOSE 2112

WORKDIR /app
COPY ./dist/linux_amd64/transcoder .
COPY ./transcoder.ex.yml ./transcoder.yml

CMD ["./transcoder"]
