# Wisteria release rules.
#
# gomobile generates a Java facade whose native symbols are bound by class
# and method name (JNI). Renaming or removing any of it breaks the engine at
# runtime with UnsatisfiedLinkError, so the whole facade is kept verbatim.
# `go.**` is gomobile's own runtime (go.Seq, go.Universe and the generated
# <pkg> bindings): the native library resolves these classes by name too.
-keep class go.** { *; }
-keep class org.wisteria.mobile.** { *; }

# Native entry points of any other JNI-backed class.
-keepclasseswithmembers class * {
    native <methods>;
}

# @JavascriptInterface methods are called by name from the WebView, not from
# Java bytecode: R8 sees them as unused and would strip or rename them.
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}
-keepattributes JavascriptInterface

# Keep the annotations and generic signatures the AndroidX shortcut and
# appcompat helpers read reflectively.
-keepattributes *Annotation*, Signature, InnerClasses, EnclosingMethod
