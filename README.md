# Grafana to Threema alert forwarder

Although Grafana has built in support for pushing alerts to Threema, it is done through the Treema Gateway, which costs about $0.15 for every alert (message + image + thumbnail x $0.05). Furthermore, the Threema Gateway costs $64 to gain access. This might be acceptable for low-traffic production system, but for personal use, it's wonky.

This project implements a Threema notification channel for Grafana using the `[go-threema](https://github.com/karalabe/go-threema)` library. This approach relies on a personal Threema account, which - although still licensed - is a small, one time expense.

Unfortunately, Grafana does not accept new notification channels implementations into their codebase. The reasoning is that they would like to ship plugin support for alerts, which has been in the [backlog since 2019](https://github.com/grafana/grafana/issues/16004). As such, this project is forced to implement a webhook server that Grafana can ping on alerts.


## Running the forwarder

The recommended way to run the Grafana to Threema forwarder is via `docker`. You can of course run it directly (it's a single-file Go code), but our assumption is that you're using some container infrastructure when monitoring things.

Building the forwarder is straightforward via docker:

```sh
$ docker build --tag grafana-threema-forwarder .
```

Running the forwarder requires a few credentials. These can be provided either via CLI flags, or - better suited to the container world - environment variables:

- `--id` or `G2T_ID_BACKUP` is the [exported Threema identity](https://github.com/karalabe/go-threema#threema-license-and-account).
- `--id.secret` or `G2T_ID_SECRET` is the encryption password for the identity.
- `--to` or `G2T_RCPT_ID` is a comma separated list of Threema IDs to send notifications to.
- `--to.pubkeys` or `G2T_RCPT_PUBKEY` is a comma separated list of [pubkeys](https://github.com/karalabe/go-threema#threema-user-directory-service) of the recipients.

The forwarder listens on port `8000`. To configure your Grafana to send alerts to it, create a new WebHook alert channel and set it to `http://address:8000`, with images enabled.

## Grafana quirks

In order to generate images, Grafana needs the image rendering plugin installed. If you are running dockerized Grafana, that image will not support it. In that case you can deploy the renderer as a separate docker container. See the [render docs](https://github.com/grafana/grafana-image-renderer) for details on how to do it.

Even with images generating, Grafana cannot embed those into webhook notifications. The solution is to configure an image provider where Grafana can upload the alert charts. In our case, hosting them locally is perfectly fine as the forwarder will retrieve them locally and send it through the Threema protocol. To do that, set the `GF_EXTERNAL_IMAGE_STORAGE_PROVIDER` environment variable on Grafana to `local`.

## Contributing

If something doesn't work, please open an issue. That said, I kind of consider this project done. There's only so many features a dumb notification forwarder can have.

## License

3-Clause BSD