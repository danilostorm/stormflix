# StormFlix for LG webOS

Small LG TV shell that opens the hosted StormFlix Web UI. Playback and remote behavior are shared with the StormFlix Web Player instead of reimplementing a second TV playback engine.

## Package

```bash
npm install
npm run check
npm run package
```

The generated `.ipk` is written to `build/`.

## Install directly on an LG TV

1. Install **Developer Mode** from the LG Content Store and sign in with an LG developer account.
2. Enable **Dev Mode Status** and **Key Server**.
3. Install the webOS CLI on the development computer, then register the TV:

```bash
ares-setup-device
```

4. Obtain the device key when requested and verify the connection:

```bash
ares-device-info -d tv
```

5. Package and install:

```bash
npm run package
ares-install -d tv build/cloud.stormflix.webos_0.1.0_all.ipk
ares-launch -d tv cloud.stormflix.webos
```

The device alias (`tv`) can be changed to the name chosen in `ares-setup-device`.
