package cloud.stormflix.app;

import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.os.Build;

import java.util.Locale;

public final class DeviceCapabilities {
    private DeviceCapabilities() {}

    public static boolean supports4kVideo() {
        return supports4kMime("video/hevc") || supports4kMime("video/avc") || supports4kMime("video/av01");
    }

    private static boolean supports4kMime(String wantedMime) {
        try {
            MediaCodecInfo[] codecs = new MediaCodecList(MediaCodecList.ALL_CODECS).getCodecInfos();
            for (MediaCodecInfo codec : codecs) {
                if (codec.isEncoder() || isSoftware(codec)) continue;
                for (String type : codec.getSupportedTypes()) {
                    if (!wantedMime.equalsIgnoreCase(type)) continue;
                    try {
                        MediaCodecInfo.CodecCapabilities caps = codec.getCapabilitiesForType(type);
                        MediaCodecInfo.VideoCapabilities video = caps.getVideoCapabilities();
                        if (video == null) continue;
                        if (video.areSizeAndRateSupported(3840, 2160, 24.0)
                                || video.areSizeAndRateSupported(3840, 2160, 30.0)
                                || video.isSizeSupported(3840, 2160)) {
                            return true;
                        }
                    } catch (Exception ignored) {
                    }
                }
            }
        } catch (Exception ignored) {
        }
        return false;
    }

    private static boolean isSoftware(MediaCodecInfo codec) {
        if (Build.VERSION.SDK_INT >= 29) return codec.isSoftwareOnly();
        String name = codec.getName().toLowerCase(Locale.ROOT);
        return name.startsWith("omx.google.")
                || name.startsWith("c2.android.")
                || name.contains("software")
                || name.contains("sw.");
    }
}
