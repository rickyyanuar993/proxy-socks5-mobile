package com.proxy.socks5frp

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat

class NotificationDismissReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        // Jika user mencoba menggeser/menghapus notifikasi saat proxy berjalan,
        // segera pasang kembali notifikasi foreground agar tetap terkunci di status bar.
        if (ProxyService.isServiceRunning) {
            val serviceIntent = Intent(context, ProxyService::class.java).apply {
                action = ProxyService.ACTION_REPOST_NOTIFICATION
            }
            try {
                ContextCompat.startForegroundService(context, serviceIntent)
            } catch (_: Exception) {}
        }
    }
}
