#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <netinet/in.h>

int main() {
    int sockfd;
    struct sockaddr_in server_addr;
    struct msghdr msg;
    struct iovec iov;
    char buffer[1024] = "Hello, World!";
    char cmsgbuf[CMSG_SPACE(sizeof(unsigned char))];
    struct cmsghdr *cmsg;

    // Create UDP socket
    sockfd = socket(AF_INET, SOCK_DGRAM, 0);
    if (sockfd < 0) {
        perror("socket");
        exit(EXIT_FAILURE);
    }

    memset(&server_addr, 0, sizeof(server_addr));
    server_addr.sin_family = AF_INET;
    server_addr.sin_port = htons(1234);
    inet_pton(AF_INET, "8.8.8.8", &server_addr.sin_addr);

    memset(&msg, 0, sizeof(msg));
    iov.iov_base = buffer;
    iov.iov_len = sizeof(buffer);
    msg.msg_name = &server_addr;
    msg.msg_namelen = sizeof(server_addr);
    msg.msg_iov = &iov;
    msg.msg_iovlen = 1;
    msg.msg_control = cmsgbuf;
    msg.msg_controllen = sizeof(cmsgbuf);

    // Set the IP_TOS ancillary data
    cmsg = CMSG_FIRSTHDR(&msg);
    cmsg->cmsg_level = IPPROTO_IP;
    cmsg->cmsg_type = IP_TOS;
    cmsg->cmsg_len = CMSG_LEN(sizeof(unsigned char));
    unsigned char tos_value = 2;
    *((unsigned char *) CMSG_DATA(cmsg)) = tos_value;

    if (sendmsg(sockfd, &msg, 0) < 0) {
        perror("sendmsg");
        exit(EXIT_FAILURE);
    }

    close(sockfd);
    return 0;
}
