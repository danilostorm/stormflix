package cloud.stormflix.app;

import android.content.Context;
import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.os.Build;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/** Builds the native /api/v1 PlaybackPlan capability request for Media3. */
public final class PlaybackCapabilities {
    private static final String VERSION = BuildConfig.VERSION_NAME;

    private static final Map<String,String> VIDEO_MIME_TO_CODEC = new LinkedHashMap<>();
    private static final Map<String,String> AUDIO_MIME_TO_CODEC = new LinkedHashMap<>();
    static {
        VIDEO_MIME_TO_CODEC.put("video/avc", "h264");
        VIDEO_MIME_TO_CODEC.put("video/hevc", "hevc");
        VIDEO_MIME_TO_CODEC.put("video/av01", "av1");
        VIDEO_MIME_TO_CODEC.put("video/x-vnd.on2.vp9", "vp9");
        VIDEO_MIME_TO_CODEC.put("video/x-vnd.on2.vp8", "vp8");
        VIDEO_MIME_TO_CODEC.put("video/mpeg2", "mpeg2");
        VIDEO_MIME_TO_CODEC.put("video/mp4v-es", "mpeg4");

        AUDIO_MIME_TO_CODEC.put("audio/mp4a-latm", "aac");
        AUDIO_MIME_TO_CODEC.put("audio/aac", "aac");
        AUDIO_MIME_TO_CODEC.put("audio/mpeg", "mp3");
        AUDIO_MIME_TO_CODEC.put("audio/ac3", "ac3");
        AUDIO_MIME_TO_CODEC.put("audio/eac3", "eac3");
        AUDIO_MIME_TO_CODEC.put("audio/eac3-joc", "eac3");
        AUDIO_MIME_TO_CODEC.put("audio/opus", "opus");
        AUDIO_MIME_TO_CODEC.put("audio/vorbis", "vorbis");
        AUDIO_MIME_TO_CODEC.put("audio/flac", "flac");
        AUDIO_MIME_TO_CODEC.put("audio/vnd.dts", "dts");
        AUDIO_MIME_TO_CODEC.put("audio/vnd.dts.hd", "dts");
        AUDIO_MIME_TO_CODEC.put("audio/true-hd", "truehd");
        AUDIO_MIME_TO_CODEC.put("audio/alac", "alac");
    }

    private PlaybackCapabilities() {}

    public static JSONObject buildRequest(Context context, SessionStore store, String playbackSessionId) throws Exception {
        Set<String> videoCodecs = new LinkedHashSet<>();
        Set<String> audioCodecs = new LinkedHashSet<>();
        JSONArray videoProfiles = new JSONArray();

        MediaCodecInfo[] codecs;
        try {
            codecs = new MediaCodecList(MediaCodecList.ALL_CODECS).getCodecInfos();
        } catch (Exception e) {
            codecs = new MediaCodecInfo[0];
        }

        for (Map.Entry<String,String> entry : VIDEO_MIME_TO_CODEC.entrySet()) {
            VideoLimits limits = inspectVideo(codecs, entry.getKey());
            if (!limits.supported) continue;
            videoCodecs.add(entry.getValue());
            JSONObject profile = new JSONObject()
                .put("codec", entry.getValue())
                .put("max_width", limits.width)
                .put("max_height", limits.height)
                .put("max_frame_rate", limits.frameRate)
                // Decoder/display HDR reporting varies heavily across Android
                // vendors. Publish observed HDR types as telemetry but keep the
                // known flag false so the server never rejects HDR on an
                // incomplete vendor capability report.
                .put("hdr_known", false)
                .put("hdr_types", limits.hdrTypes);
            videoProfiles.put(profile);
        }

        for (Map.Entry<String,String> entry : AUDIO_MIME_TO_CODEC.entrySet()) {
            if (hasDecoder(codecs, entry.getKey())) audioCodecs.add(entry.getValue());
        }

        // Android's platform compatibility definition guarantees baseline AVC
        // and AAC decoders on the API levels StormFlix supports. Keep these in
        // the request even on vendor builds that hide codec enumeration details.
        videoCodecs.add("h264");
        audioCodecs.add("aac");

        JSONArray containers = array("mp4", "mkv", "webm", "ts", "mpegts", "m2ts", "flv", "ogg");
        JSONObject capabilities = new JSONObject()
            .put("containers", containers)
            .put("video_codecs", new JSONArray(new ArrayList<>(videoCodecs)))
            .put("audio_codecs", new JSONArray(new ArrayList<>(audioCodecs)))
            .put("video_profiles", videoProfiles)
            .put("subtitle_formats", array("vtt"))
            .put("allow_remux", true)
            .put("allow_audio_compatibility", audioCodecs.contains("aac"))
            .put("native_audio_track_selection", true)
            .put("picture_in_picture", Build.VERSION.SDK_INT >= 26 && !RemoteUi.isTelevision(context))
            .put("media_session", true);

        JSONObject request = new JSONObject()
            .put("client_kind", RemoteUi.isTelevision(context) ? "tv" : "android")
            .put("client_name", RemoteUi.isTelevision(context) ? "StormFlix Android TV / Fire TV" : "StormFlix Android")
            .put("client_version", VERSION)
            .put("capabilities", capabilities)
            .put("preferred_audio_language", store.preferredAudio());
        if (playbackSessionId != null && !playbackSessionId.trim().isEmpty()) {
            request.put("playback_session_id", playbackSessionId.trim());
        }
        return request;
    }

    private static VideoLimits inspectVideo(MediaCodecInfo[] codecs, String mime) {
        VideoLimits best = new VideoLimits();
        for (MediaCodecInfo codec : codecs) {
            if (codec.isEncoder() || !supportsType(codec, mime)) continue;
            best.supported = true;
            try {
                MediaCodecInfo.CodecCapabilities caps = codec.getCapabilitiesForType(mime);
                addHdrTypes(caps, mime, best.hdrTypes);
                MediaCodecInfo.VideoCapabilities video = caps.getVideoCapabilities();
                if (video == null) continue;
                VideoLimits candidate = commonVideoLimits(video);
                candidate.supported = true;
                if (candidate.area() > best.area() || (candidate.area() == best.area() && candidate.frameRate > best.frameRate)) {
                    Set<String> hdr = new LinkedHashSet<>(best.hdrTypes);
                    best = candidate;
                    best.hdrTypes.addAll(hdr);
                }
            } catch (Exception ignored) {
            }
        }
        return best;
    }

    private static VideoLimits commonVideoLimits(MediaCodecInfo.VideoCapabilities video) {
        int[][] sizes = {
            {7680,4320}, {4096,2160}, {3840,2160}, {2560,1440},
            {1920,1080}, {1280,720}, {720,480}
        };
        double[] rates = {120, 60, 50, 30, 25, 24};
        VideoLimits result = new VideoLimits();
        for (int[] size : sizes) {
            boolean supported = false;
            for (double rate : rates) {
                try {
                    if (video.areSizeAndRateSupported(size[0], size[1], rate)) {
                        if (!supported) {
                            result.width = size[0];
                            result.height = size[1];
                            supported = true;
                        }
                        result.frameRate = Math.max(result.frameRate, rate);
                    }
                } catch (Exception ignored) {
                }
            }
            if (supported) return result;
            try {
                if (video.isSizeSupported(size[0], size[1])) {
                    result.width = size[0];
                    result.height = size[1];
                    result.frameRate = 24;
                    return result;
                }
            } catch (Exception ignored) {
            }
        }
        return result;
    }

    private static void addHdrTypes(MediaCodecInfo.CodecCapabilities caps, String mime, Set<String> out) {
        if (caps == null || caps.profileLevels == null) return;
        for (MediaCodecInfo.CodecProfileLevel level : caps.profileLevels) {
            int profile = level.profile;
            if ("video/hevc".equalsIgnoreCase(mime)) {
                if (profile == MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10) out.add("hdr10");
                if (profile == MediaCodecInfo.CodecProfileLevel.HEVCProfileMain10HDR10Plus) out.add("hdr10plus");
            } else if ("video/av01".equalsIgnoreCase(mime)) {
                if (profile == MediaCodecInfo.CodecProfileLevel.AV1ProfileMain10HDR10) out.add("hdr10");
                if (profile == MediaCodecInfo.CodecProfileLevel.AV1ProfileMain10HDR10Plus) out.add("hdr10plus");
            } else if ("video/x-vnd.on2.vp9".equalsIgnoreCase(mime)) {
                if (profile == MediaCodecInfo.CodecProfileLevel.VP9Profile2HDR || profile == MediaCodecInfo.CodecProfileLevel.VP9Profile3HDR) out.add("hdr10");
                if (profile == MediaCodecInfo.CodecProfileLevel.VP9Profile2HDR10Plus || profile == MediaCodecInfo.CodecProfileLevel.VP9Profile3HDR10Plus) out.add("hdr10plus");
            }
        }
    }

    private static boolean hasDecoder(MediaCodecInfo[] codecs, String mime) {
        for (MediaCodecInfo codec : codecs) {
            if (!codec.isEncoder() && supportsType(codec, mime)) return true;
        }
        return false;
    }

    private static boolean supportsType(MediaCodecInfo codec, String mime) {
        try {
            for (String type : codec.getSupportedTypes()) {
                if (mime.equalsIgnoreCase(type)) return true;
            }
        } catch (Exception ignored) {
        }
        return false;
    }

    private static JSONArray array(String... values) {
        JSONArray out = new JSONArray();
        for (String value : values) out.put(value);
        return out;
    }

    private static final class VideoLimits {
        boolean supported;
        int width;
        int height;
        double frameRate;
        final Set<String> hdrTypes = new LinkedHashSet<>();
        long area() { return (long) width * (long) height; }
    }
}
