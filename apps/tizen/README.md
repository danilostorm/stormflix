# StormFlix for Samsung Tizen

Small Samsung TV shell that hands playback/navigation to the hosted StormFlix Web UI. The server-side `tv-remote.js` remains the shared remote-control layer, so Fire TV, Tizen and webOS use the same semantic commands.

## Build and install on a Samsung TV

1. Install Tizen Studio with the Samsung TV extensions and Certificate Manager.
2. On the TV open Apps, enter `12345`, enable Developer Mode, set the development computer IP and restart the TV.
3. Create/select a Samsung certificate profile in Tizen Studio.
4. From this directory run:

```bash
tizen build-web -- .
tizen package -t wgt -o build -- .buildResult
```

5. Find the TV target with `sdb devices` and install the generated WGT:

```bash
tizen install -n build/StormFlix.wgt -t <TV_TARGET>
```

The exact WGT filename may include the project name assigned by Tizen Studio. Samsung developer signing is intentionally not committed to this repository; each developer/TV uses its own certificate profile.
