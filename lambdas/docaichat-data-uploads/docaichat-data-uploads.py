import json
import boto3
import base64
import os
from datetime import datetime

s3 = boto3.client('s3')
bucket_name = os.getenv("AICHAT_DATA_UPLOAD_BUCKET")

RESP_HEADERS = {
    "Access-Control-Allow-Origin":  "*",
    "Access-Control-Allow-Methods": "*",
    "Access-Control-Allow-Headers": "X-Amz-Date,X-Api-Key,X-Amz-Security-Token,X-Requested-With,X-Auth-Token,Referer,User-Agent,Origin,Content-Type,Authorization,Accept,Access-Control-Allow-Methods,Access-Control-Allow-Origin,Access-Control-Allow-Headers"
}


def lambda_handler(event, context):
    # Ensure the event contains necessary data
    if 'body' not in event:
        return {
            'headers': RESP_HEADERS,
            'statusCode': 400,
            'body': json.dumps('Missing request body')
        }
    
    # Parse request body
    try:
        request_body = json.loads(event['body'])
    except json.JSONDecodeError:
        return {
            'headers': RESP_HEADERS,
            'statusCode': 400,
            'body': json.dumps('Invalid JSON format in request body')
        }
    
    # Ensure the request body contains necessary data
    if 'file_content' not in request_body:
        return {
            'headers': RESP_HEADERS,
            'statusCode': 400,
            'body': json.dumps('Request body must include "file_content"')
        }
    
    # Decode base64-encoded file content
    try:
        file_content = base64.b64decode(request_body['file_content'])
    except Exception as e:
        return {
            'headers': RESP_HEADERS,
            'statusCode': 400,
            'body': json.dumps('Error decoding base64 file content: ' + str(e))
        }
    
    # Upload file to S3
    try:
        s3.put_object(
            Bucket=bucket_name,
            Key=generate_file_name(),
            Body=file_content
        )
    except Exception as e:
        return {
            'headers': RESP_HEADERS,
            'statusCode': 500,
            'body': json.dumps('Error uploading file to S3: ' + str(e))
        }
    
    return {
        'headers': RESP_HEADERS,
        'statusCode': 200,
        'body': json.dumps('File uploaded successfully to S3')
    }


def generate_file_name():
    # Get current date and time
    current_datetime = datetime.now()

    # Format date and time
    date_str = current_datetime.strftime("%Y-%m-%d")
    time_str = current_datetime.strftime("%H-%M-%S-%f")[:-3]  # Remove last 3 digits (milliseconds)

    # Generate file name
    file_name = f"aichat-data_{date_str}_{time_str}.txt"

    return file_name