FROM alpine
COPY ./build/* /
COPY ./ssh/* /root/.ssh/
ENV CGO_ENABLED=0
RUN chmod +x /dnam && chmod 600 /root/.ssh/id_ed25519
CMD ["/dnam"]