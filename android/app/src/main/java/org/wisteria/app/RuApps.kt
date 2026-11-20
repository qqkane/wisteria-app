package org.wisteria.app

import android.content.Context

/**
 * The 30 popular Russian apps audited for VPN detection / surveillance
 * (RKS Global report, April 2026 — the package table in its methodology
 * section). Offered as a one-tap bypass preset for the per-app split
 * tunnel: banks and RU services see the real IP and work natively, while
 * the rest of the device stays behind the tunnel.
 *
 * Package names are stable identifiers; a renamed app (T-Bank was
 * Tinkoff, Gosuslugi ships as ru.rostel) keeps its old package, so the
 * list stays valid across rebrands.
 */
object RuApps {

    /** package → display name, in report order. */
    val ALL: List<Pair<String, String>> = listOf(
        "ru.sberbankmobile" to "Сбербанк Онлайн",
        "com.idamob.tinkoff.android" to "Т-Банк",
        "ru.vtb24.mobilebanking.android" to "ВТБ Онлайн",
        "ru.alfabank.mobile.android" to "Альфа-Банк",
        "ru.nspk.mirpay" to "Mir Pay",
        "com.wildberries.ru" to "Wildberries",
        "ru.ozon.app.android" to "Ozon",
        "ru.megamarket.marketplace" to "MegaMarket",
        "ru.beru.android" to "Яндекс Маркет",
        "com.avito.android" to "Avito",
        "ru.sbcs.store" to "Самокат",
        "ru.foodfox.client" to "Яндекс Еда",
        "com.vkontakte.android" to "ВКонтакте",
        "ru.ok.android" to "Одноклассники",
        "com.vk.vkvideo" to "VK Видео",
        "com.uma.musicvk" to "VK Музыка",
        "ru.oneme.app" to "MAX",
        "ru.mail.mailapp" to "Почта Mail.ru",
        "com.yandex.browser" to "Яндекс Браузер",
        "ru.yandex.yandexmaps" to "Яндекс Карты",
        "ru.yandex.music" to "Yandex Music",
        "ru.yandex.taxi" to "Яндекс Go",
        "ru.kinopoisk" to "Кинопоиск",
        "ru.zen.android" to "Дзен",
        "ru.rutube.app" to "Rutube",
        "ru.dublgis.dgismobile" to "2ГИС",
        "ru.rostel" to "Госуслуги",
        "ru.megafon.mlk" to "МегаФон",
        "ru.mts.mymts" to "Мой МТС",
        "ru.vk.store" to "RuStore",
    )

    /** Only the packages actually present on this device. */
    fun installed(context: Context): List<Pair<String, String>> {
        val pm = context.packageManager
        return ALL.filter { (pkg, _) ->
            try {
                pm.getPackageInfo(pkg, 0)
                true
            } catch (_: Exception) {
                false
            }
        }
    }
}