package app.firlab.pttools

import android.app.Activity
import android.content.ClipData
import android.content.ContentValues
import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Build
import android.provider.MediaStore
import androidx.core.content.FileProvider
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    private var pendingSave: Pair<ByteArray, MethodChannel.Result>? = null
    private val saveRequest = 4319

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "app.firlab.pttools/images")
            .setMethodCallHandler { call, result ->
                if (call.method != "save" && call.method != "share") {
                    result.notImplemented()
                    return@setMethodCallHandler
                }
                val bytes = call.argument<ByteArray>("bytes")
                val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                if (bytes != null && bytes.size <= 16 * 1024 * 1024) {
                    BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
                }
                if (bytes == null || bounds.outMimeType != "image/png" ||
                    bounds.outWidth !in 1..4096 || bounds.outHeight !in 1..16384 ||
                    bounds.outWidth.toLong() * bounds.outHeight > 16_000_000) {
                    result.error("invalid_image", "Invalid or oversized PNG", null)
                    return@setMethodCallHandler
                }
                try {
                    if (call.method == "share") {
                        val dir = File(cacheDir, "shared-posters").apply { mkdirs() }
                        // Keep newly shared files long enough for receiving applications to read.
                        dir.listFiles()?.filter { it.lastModified() < System.currentTimeMillis() - 86_400_000 }
                            ?.forEach { it.delete() }
                        val file = File.createTempFile("pt-stats-", ".png", dir)
                        file.writeBytes(bytes)
                        val uri = FileProvider.getUriForFile(this, "$packageName.posters", file)
                        val intent = Intent(Intent.ACTION_SEND).apply {
                            type = "image/png"
                            putExtra(Intent.EXTRA_STREAM, uri)
                            clipData = ClipData.newRawUri("PT statistics", uri)
                            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                        }
                        startActivity(Intent.createChooser(intent, null))
                        // Launching the chooser never claims that a recipient received the image.
                        result.success(false)
                    } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                        val values = ContentValues().apply {
                            put(MediaStore.Images.Media.DISPLAY_NAME, "pt-stats-${System.currentTimeMillis()}.png")
                            put(MediaStore.Images.Media.MIME_TYPE, "image/png")
                            put(MediaStore.Images.Media.RELATIVE_PATH, "Pictures/pt-tools")
                            put(MediaStore.Images.Media.IS_PENDING, 1)
                        }
                        val uri = contentResolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values)
                            ?: throw IllegalStateException("Cannot create image")
                        try {
                            val stream = contentResolver.openOutputStream(uri)
                                ?: throw IllegalStateException("Cannot open image")
                            stream.use { it.write(bytes) }
                            values.clear()
                            values.put(MediaStore.Images.Media.IS_PENDING, 0)
                            contentResolver.update(uri, values, null, null)
                            result.success(true)
                        } catch (error: Exception) {
                            contentResolver.delete(uri, null, null)
                            throw error
                        }
                    } else {
                        if (pendingSave != null) {
                            result.error("busy", "A save dialog is already open", null)
                            return@setMethodCallHandler
                        }
                        pendingSave = Pair(bytes, result)
                        try {
                            startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                                addCategory(Intent.CATEGORY_OPENABLE)
                                type = "image/png"
                                putExtra(Intent.EXTRA_TITLE, "pt-stats-${System.currentTimeMillis()}.png")
                            }, saveRequest)
                        } catch (error: Exception) {
                            pendingSave = null
                            throw error
                        }
                    }
                } catch (error: Exception) {
                    result.error("image_failed", "Unable to save or share image", null)
                }
            }
    }

    @Deprecated("Android activity result callback required by the Flutter host")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != saveRequest) return
        val pending = pendingSave ?: return
        pendingSave = null
        val uri: Uri? = data?.data
        if (resultCode != Activity.RESULT_OK || uri == null) {
            pending.second.success(false)
            return
        }
        try {
            val stream = contentResolver.openOutputStream(uri)
                ?: throw IllegalStateException("Cannot open destination")
            stream.use { it.write(pending.first) }
            pending.second.success(true)
        } catch (error: Exception) {
            pending.second.error("image_failed", "Unable to save image", null)
        }
    }

    override fun onDestroy() {
        pendingSave?.second?.error("cancelled", "Activity closed", null)
        pendingSave = null
        super.onDestroy()
    }
}
