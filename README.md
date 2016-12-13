# hkvisor

Notification and monitoring application for Hikvision IP cameras

*This is still a work in progress. See todo for things to come.*

## Why?

I do not trust the firmware running on Hikvision cameras. I also do not want to
run their NVR (nor any others) when the cameras themselves are the smarts that
detect motion and control when to record. I currently place these devices on their
own subnet with no access to any other networks. This application runs on a device
that does have a leg into this subnet and can notify me when motion events occur.

## How?

This program connects to the notification event stream of each camera and parses
events sent. When an active event occurs, it will notify you via the configured
notification method.

