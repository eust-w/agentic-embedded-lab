#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#import <ImageIO/ImageIO.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>

static NSString *AetherStringAttribute(AXUIElementRef element, CFStringRef attribute) {
    CFTypeRef value = NULL;
    if (AXUIElementCopyAttributeValue(element, attribute, &value) != kAXErrorSuccess || value == NULL) return @"";
    NSString *result = @"";
    if (CFGetTypeID(value) == CFStringGetTypeID()) result = [(__bridge NSString *)value copy];
    CFRelease(value);
    return result;
}

static NSDictionary *AetherElementDictionary(AXUIElementRef element, int *remaining, int depth) {
    if (*remaining <= 0 || depth > 10) return nil;
    (*remaining)--;
    NSMutableDictionary *result = [NSMutableDictionary dictionary];
    NSString *role = AetherStringAttribute(element, kAXRoleAttribute);
    NSString *subrole = AetherStringAttribute(element, kAXSubroleAttribute);
    NSString *title = AetherStringAttribute(element, kAXTitleAttribute);
    NSString *description = AetherStringAttribute(element, kAXDescriptionAttribute);
    if (role.length) result[@"role"] = role;
    if (subrole.length) result[@"subrole"] = subrole;
    if (title.length) result[@"title"] = title;
    if (description.length) result[@"description"] = description;

    CFTypeRef position = NULL;
    CFTypeRef size = NULL;
    CGPoint point = CGPointZero;
    CGSize dimensions = CGSizeZero;
    if (AXUIElementCopyAttributeValue(element, kAXPositionAttribute, &position) == kAXErrorSuccess && position) {
        AXValueGetValue(position, kAXValueCGPointType, &point);
        CFRelease(position);
    }
    if (AXUIElementCopyAttributeValue(element, kAXSizeAttribute, &size) == kAXErrorSuccess && size) {
        AXValueGetValue(size, kAXValueCGSizeType, &dimensions);
        CFRelease(size);
    }
    result[@"frame"] = @{@"x": @(point.x), @"y": @(point.y), @"width": @(dimensions.width), @"height": @(dimensions.height)};

    CFTypeRef childrenValue = NULL;
    if (*remaining > 0 && AXUIElementCopyAttributeValue(element, kAXChildrenAttribute, &childrenValue) == kAXErrorSuccess && childrenValue) {
        NSArray *children = (__bridge NSArray *)childrenValue;
        NSMutableArray *serialized = [NSMutableArray array];
        for (id child in children) {
            NSDictionary *item = AetherElementDictionary((__bridge AXUIElementRef)child, remaining, depth + 1);
            if (item) [serialized addObject:item];
            if (*remaining <= 0) break;
        }
        if (serialized.count) result[@"children"] = serialized;
        CFRelease(childrenValue);
    }
    return result;
}

char *aether_frontmost_bundle_id(void) {
    NSString *bundleID = NSWorkspace.sharedWorkspace.frontmostApplication.bundleIdentifier;
    return bundleID.length ? strdup(bundleID.UTF8String) : NULL;
}

bool aether_focused_secure(void) {
    NSRunningApplication *application = NSWorkspace.sharedWorkspace.frontmostApplication;
    if (!application) return true;
    AXUIElementRef app = AXUIElementCreateApplication(application.processIdentifier);
    CFTypeRef focused = NULL;
    bool secure = true;
    if (AXUIElementCopyAttributeValue(app, kAXFocusedUIElementAttribute, &focused) == kAXErrorSuccess && focused) {
        NSString *role = AetherStringAttribute((AXUIElementRef)focused, kAXRoleAttribute);
        NSString *subrole = AetherStringAttribute((AXUIElementRef)focused, kAXSubroleAttribute);
        secure = [role isEqualToString:@"AXSecureTextField"] || [subrole isEqualToString:@"AXSecureTextField"];
        CFRelease(focused);
    }
    CFRelease(app);
    return secure;
}

char *aether_ax_snapshot_json(int limit) {
    NSRunningApplication *application = NSWorkspace.sharedWorkspace.frontmostApplication;
    if (!application) return NULL;
    AXUIElementRef app = AXUIElementCreateApplication(application.processIdentifier);
    int remaining = limit;
    NSDictionary *tree = AetherElementDictionary(app, &remaining, 0);
    CFRelease(app);
    if (!tree) return NULL;
    NSData *data = [NSJSONSerialization dataWithJSONObject:tree options:0 error:nil];
    if (!data) return NULL;
    NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
    return json.length ? strdup(json.UTF8String) : NULL;
}

unsigned char *aether_screen_png(size_t *length) {
    if (length) *length = 0;
    __block CGImageRef image = NULL;
    dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
    [SCShareableContent getShareableContentExcludingDesktopWindows:NO onScreenWindowsOnly:YES completionHandler:^(SCShareableContent *content, NSError *error) {
        SCDisplay *display = content.displays.firstObject;
        if (error || !display) { dispatch_semaphore_signal(semaphore); return; }
        SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
        SCStreamConfiguration *configuration = [[SCStreamConfiguration alloc] init];
        configuration.width = display.width;
        configuration.height = display.height;
        configuration.showsCursor = YES;
        [SCScreenshotManager captureImageWithFilter:filter configuration:configuration completionHandler:^(CGImageRef captured, NSError *captureError) {
            if (!captureError && captured) image = CGImageRetain(captured);
            dispatch_semaphore_signal(semaphore);
        }];
    }];
    if (dispatch_semaphore_wait(semaphore, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC)) != 0) return NULL;
    if (!image) return NULL;
    CFMutableDataRef data = CFDataCreateMutable(NULL, 0);
    CGImageDestinationRef destination = CGImageDestinationCreateWithData(data, CFSTR("public.png"), 1, NULL);
    if (!destination) { CGImageRelease(image); CFRelease(data); return NULL; }
    CGImageDestinationAddImage(destination, image, NULL);
    bool ok = CGImageDestinationFinalize(destination);
    CFRelease(destination);
    CGImageRelease(image);
    if (!ok) { CFRelease(data); return NULL; }
    CFIndex size = CFDataGetLength(data);
    unsigned char *buffer = malloc((size_t)size);
    if (buffer) memcpy(buffer, CFDataGetBytePtr(data), (size_t)size);
    CFRelease(data);
    if (buffer && length) *length = (size_t)size;
    return buffer;
}

void aether_free_buffer(void *buffer) { free(buffer); }
